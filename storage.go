package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config" // "config" 충돌 방지 위해 별칭 사용
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/pro200/go-utils"
)

type Config struct {
	Endpoint        string
	Region          string // default: auto
	AccessKeyID     string
	SecretAccessKey string
}

type Options struct {
	Headers     map[string]string
	ContentType string
}

type SType string

type Storage struct {
	config        Config
	client        *s3.Client
	presignClient *s3.PresignClient
}

func New(config Config) (*Storage, error) {
	if config.Endpoint == "" {
		return nil, errors.New("missing endpoint: <account-id>.r2.cloudflarestorage.com or s3.<region>.backblazeb2.com")
	}

	if !strings.HasPrefix(config.Endpoint, "http://") && !strings.HasPrefix(config.Endpoint, "https://") {
		config.Endpoint = "https://" + config.Endpoint
	}

	if config.Region == "" && strings.Contains(config.Endpoint, "backblazeb2") {
		parts := strings.Split(config.Endpoint, ".")
		config.Region = parts[1]
	}

	if config.Region == "" {
		config.Region = "auto"
	}

	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(config.AccessKeyID, config.SecretAccessKey, "")),
		awsConfig.WithRegion(config.Region),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(config.Endpoint)
	})

	return &Storage{
		config:        config,
		client:        client,
		presignClient: s3.NewPresignClient(client),
	}, nil
}

func (s *Storage) Info(bucket, key string) (*s3.HeadObjectOutput, error) {
	return s.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
}

func (s *Storage) List(bucket, prefix string, length int, token ...string) (list []string, nextToken string, err error) {
	// up to 1,000 keys
	if length > 1000 {
		length = 1000
	}

	options := s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(int32(length)),
	}

	// ContinuationToken
	// A token to specify where to start paginating. This is the NextContinuationToken from a previously truncated response.
	if len(token) > 0 {
		options.ContinuationToken = aws.String(token[0])
	}

	output, err := s.client.ListObjectsV2(context.TODO(), &options)
	if err != nil {
		return list, nextToken, err
	}

	for _, obj := range output.Contents {
		list = append(list, aws.ToString(obj.Key))
	}

	nextToken = aws.ToString(output.NextContinuationToken)
	return list, nextToken, nil
}

func (s *Storage) Upload(bucket, key, origin string, options ...Options) error {
	var (
		err      error
		resp     *http.Response
		file     *os.File
		size     int
		isRemote = strings.HasPrefix(origin, "https://")
	)

	var opt = new(Options)
	if len(options) > 0 {
		opt = &options[0]
	}

	// remote 파일 스트림
	if isRemote {
		req, _ := http.NewRequest("GET", origin, nil)

		// set headers
		for key, value := range opt.Headers {
			req.Header.Set(key, value)
		}

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// befor using resp.Body, check resp.StatusCode
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("origin download failed: %s", resp.Status)
		}

		// get content type from response if not set
		if opt.ContentType == "" {
			opt.ContentType = resp.Header.Get("Content-Type")
		}

		size = int(resp.ContentLength)
	} else {
		file, err = os.Open(origin)
		if err != nil {
			return err
		}
		defer file.Close()
		stat, _ := file.Stat()
		size = int(stat.Size())
	}

	// content type from file extension if not set
	if opt.ContentType == "" {
		opt.ContentType = utils.ContentType(origin)
	}

	if size == 0 {
		return errors.New("zero size file")
	}

	putObject := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(opt.ContentType),
	}

	// remote url
	if isRemote {
		putObject.Body = resp.Body
	}

	uploader := manager.NewUploader(s.client)
	_, err = uploader.Upload(context.TODO(), putObject)
	if err != nil {
		return err
	}

	// 업로드된 용량 비교
	result, err := s.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}

	// TODO: 업로드 실패한 파일을 삭제
	if size != int(*result.ContentLength) {
		return errors.New("upload failed")
	}

	return nil
}

func (s *Storage) Delete(bucket, key string) error {
	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return err
}

func (s *Storage) Download(bucket, key, targetPath string) error {
	fd, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("cannot create file: %w", err)
	}
	defer fd.Close()

	downloader := manager.NewDownloader(s.client)
	_, err = downloader.Download(context.TODO(), fd,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
	return err
}

func (s *Storage) PresignGet(bucket, key string, ttl time.Duration) (string, error) {
	res, err := s.presignClient.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return res.URL, nil
}

func (s *Storage) PresignPut(bucket, key string, ttl time.Duration) (string, error) {
	res, err := s.presignClient.PresignPutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return res.URL, nil
}
