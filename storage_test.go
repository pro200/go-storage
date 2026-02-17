package storage_test

import (
	"testing"

	"github.com/pro200/go-config"
	"github.com/pro200/go-storage"
)

func TestStorage(t *testing.T) {
	cfg, err := config.New()
	if err != nil {
		t.Error(err)
	}

	b2, err := storage.New(storage.Config{
		Endpoint:        cfg.Get("ENDPOINT"),
		AccessKeyID:     cfg.Get("ACCESS_KEY_ID"),
		SecretAccessKey: cfg.Get("SECRET_ACCESS_KEY"),
	})
	if err != nil {
		t.Error("연결 실패:", err)
	}

	result, _, err := b2.List("diskn-test", "", 10)
	if err != nil {
		t.Error("리스트 실패:", err)
	}

	if len(result) == 0 {
		t.Error("파일이 존재하지 않습니다")
	}
}
