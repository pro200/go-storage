package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

type SType string

const (
	r2        SType = "r2"
	backblaze SType = "backblazeb2"
	bunnyCDN  SType = "bunnycdn"
	etc       SType = "etc"
)

type Storage struct {
	config        Config
	client        *s3.Client
	presignClient *s3.PresignClient
}

func NewStorage(config Config) (*Storage, error) {
	if config.Endpoint == "" {
		return nil, errors.New("missing endpoint: <account-id>.r2.cloudflarestorage.com or s3.<region>.backblazeb2.com or storage.bunnycdn.com")
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

	// bunnycdn은 기본 S3 클라이언트 사용 안함
	if strings.Contains(config.Endpoint, "bunnycdn") {
		return &Storage{
			config: config,
		}, nil
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

func (s *Storage) Type() SType {
	if strings.Contains(s.config.Endpoint, "cloudflarestorage") {
		return r2
	} else if strings.Contains(s.config.Endpoint, "backblazeb2") {
		return backblaze
	} else if strings.Contains(s.config.Endpoint, "bunnycdn") {
		return bunnyCDN
	}
	return etc
}

func (s *Storage) Info(bucket, key string) (*s3.HeadObjectOutput, error) {
	if s.Type() == bunnyCDN {
		return nil, errors.New("bunnycdn storage does not support Info operation")
	}

	return s.client.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
}

func (s *Storage) List(bucket, prefix string, length int, token ...string) (list []string, nextToken string, err error) {
	if s.Type() == bunnyCDN {
		return list, nextToken, errors.New("bunnycdn storage does not support List operation")
	}

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

func (s *Storage) Upload(bucket, path, key string, forceType ...string) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if len(file) == 0 {
		return errors.New("zero size file")
	}

	contentType := utils.ContentType(path)
	if len(forceType) > 0 {
		contentType = forceType[0]
	}

	if s.Type() == bunnyCDN {
		url := fmt.Sprintf("%s/%s/%s", s.config.Endpoint, bucket, key)
		req, err := http.NewRequest("PUT", url, bytes.NewReader(file))
		if err != nil {
			return err
		}

		req.Header.Set("AccessKey", s.config.SecretAccessKey)
		req.Header.Set("Content-Type", contentType)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode >= 300 {
			body, _ := io.ReadAll(res.Body)
			return fmt.Errorf("upload failed: %s", string(body))
		}
		return nil
	}

	uploader := manager.NewUploader(s.client)
	_, err = uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(file),
		ContentType: aws.String(contentType),
	})
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
	if len(file) != int(*result.ContentLength) {
		return errors.New("upload failed")
	}

	return nil
}

func (s *Storage) Delete(bucket, key string) error {
	if s.Type() == bunnyCDN {
		url := fmt.Sprintf("%s/%s/%s", s.config.Endpoint, bucket, key)
		req, _ := http.NewRequest("DELETE", url, nil)
		req.Header.Set("AccessKey", s.config.SecretAccessKey)

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode >= 300 {
			body, _ := io.ReadAll(res.Body)
			return fmt.Errorf("delete failed: %s", string(body))
		}
		return nil
	}

	_, err := s.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return err
}

func (s *Storage) Download(bucket, key, targetPath string) error {
	if s.Type() == bunnyCDN {
		url := fmt.Sprintf("%s/%s/%s", s.config.Endpoint, bucket, key)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}

		req.Header.Set("AccessKey", s.config.SecretAccessKey)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed, status: %d", res.StatusCode)
		}

		out, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("cannot create file: %w", err)
		}
		defer out.Close()

		// Download (stream copy)
		_, err = io.Copy(out, res.Body)
		if err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		return nil
	}

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
	if s.Type() == bunnyCDN {
		return "", errors.New("bunnycdn storage does not support Presign operation")
	}

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
	if s.Type() == bunnyCDN {
		return "", errors.New("bunnycdn storage does not support Presign operation")
	}

	res, err := s.presignClient.PresignPutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return res.URL, nil
}
