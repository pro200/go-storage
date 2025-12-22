# Storage Package

S3 호환 스토리지(AWS S3, Cloudflare R2, Backblaze B2 등)를 **단일 인터페이스**로 다루기 위한 Go 패키지입니다.

파일 업로드/다운로드, 객체 조회, 목록 조회, 삭제, Presigned URL 생성 기능을 제공합니다.

---

## 지원 스토리지

이 패키지는 **AWS SDK v2** 기반으로 동작하며, 다음과 같은 S3 호환 스토리지를 지원합니다.

- **AWS S3**
- **Cloudflare R2**
- **Backblaze B2 (S3 Compatible API)**
- 기타 S3 API 호환 스토리지

---

## 주요 기능

- 객체 정보 조회 (`HeadObject`)
- 객체 목록 조회 (`ListObjectsV2`)
- 로컬 파일 업로드
- 원격(URL) 파일 업로드
- 파일 다운로드
- 객체 삭제
- Presigned GET / PUT URL 생성

---

## 설치

```bash
go get github.com/pro200/go-storage
```

---

## 기본 구조

### Config

```go
type Config struct {
    Endpoint        string
    Region          string // default: auto
    AccessKeyID     string
    SecretAccessKey string
}
```

| 필드 | 설명 |
|---|---|
| Endpoint | S3 호환 엔드포인트 주소 |
| Region | 리전 (비워두면 auto) |
| AccessKeyID | 액세스 키 |
| SecretAccessKey | 시크릿 키 |

#### Endpoint 예시

- Cloudflare R2  
  `https://<account-id>.r2.cloudflarestorage.com`

- Backblaze B2  
  `https://s3.<region>.backblazeb2.com`

---

### Options (업로드 옵션)

```go
type Options struct {
    Headers     map[string]string
    ContentType string
}
```

| 필드 | 설명 |
|---|---|
| Headers | 원격 파일 다운로드 시 사용할 HTTP 헤더 |
| ContentType | 업로드 시 사용할 Content-Type |

---

## 초기화

```go
store, err := storage.NewStorage(storage.Config{
    Endpoint:        "<endpoint>",
    Region:          "auto",
    AccessKeyID:     "ACCESS_KEY",
    SecretAccessKey: "SECRET_KEY",
})
if err != nil {
    panic(err)
}
```

### 동작 특징

- Endpoint에 `http(s)://`가 없으면 자동으로 `https://`를 추가합니다.
- Backblaze B2 사용 시 Endpoint에서 Region을 자동 추출합니다.
- Region이 비어 있으면 기본값은 `auto`입니다.

---

## API 설명

### 객체 정보 조회

```go
info, err := store.Info("bucket", "path/file.jpg")
```

- 내부적으로 `HeadObject` 호출

---

### 객체 목록 조회

```go
list, nextToken, err := store.List("bucket", "prefix/", 100)
```

| 파라미터 | 설명 |
|---|---|
| bucket | 버킷 이름 |
| prefix | 객체 prefix |
| length | 최대 반환 개수 (최대 1000) |
| token | ContinuationToken (옵션) |

- `nextToken`이 비어있지 않으면 다음 페이지 존재

---

### 파일 업로드 (로컬 파일)

```go
err := store.Upload("bucket", "path/file.jpg", "/local/file.jpg")
```

---

### 파일 업로드 (원격 URL)

```go
err := store.Upload(
    "bucket",
    "path/file.jpg",
    "https://example.com/file.jpg",
    storage.Options{
        Headers: map[string]string{
            "Authorization": "Bearer token",
        },
    },
)
```

#### 업로드 특징

- 로컬 파일 또는 HTTP(S) URL 모두 지원
- Content-Type 미지정 시 자동 추론
- 업로드 후 실제 저장된 파일 크기 검증
- 크기가 0인 파일은 업로드 거부

---

### 파일 다운로드

```go
err := store.Download("bucket", "path/file.jpg", "/tmp/file.jpg")
```

---

### 객체 삭제

```go
err := store.Delete("bucket", "path/file.jpg")
```

---

### Presigned GET URL 생성

```go
url, err := store.PresignGet(
    "bucket",
    "path/file.jpg",
    10*time.Minute,
)
```

---

### Presigned PUT URL 생성

```go
url, err := store.PresignPut(
    "bucket",
    "path/file.jpg",
    10*time.Minute,
)
```

---

## 주의 사항

- AWS SDK v2 기반이므로 Go 1.18+ 권장
- Presigned URL TTL은 스토리지 정책에 따라 제한될 수 있음
- 업로드 검증은 `Content-Length` 기준 비교
- 업로드 실패 시 객체 자동 삭제는 아직 구현되지 않음 (TODO)

---

## 사용 라이브러리

- `github.com/aws/aws-sdk-go-v2`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `github.com/aws/aws-sdk-go-v2/feature/s3/manager`
- `github.com/pro200/go-utils`
