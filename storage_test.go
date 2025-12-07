package storage_test

import (
	"slices"
	"testing"

	"github.com/pro200/go-env"
	"github.com/pro200/go-storage"
)

func TestStorage(t *testing.T) {
	config, err := env.NewEnv()
	if err != nil {
		t.Error(err)
	}

	b2, err := storage.NewStorage(storage.Config{
		Endpoint:        config.Get("ENDPOINT"),
		AccessKeyID:     config.Get("ACCESS_KEY_ID"),
		SecretAccessKey: config.Get("SECRET_ACCESS_KEY"),
	})
	if err != nil {
		t.Error("연결 실패:", err)
	}

	result, _, err := b2.List("diskn-test", "", 10)
	if err != nil {
		t.Error("리스트 실패:", err)
	}

	if !slices.Contains(result, "rabbit.jpg") {
		t.Error("파일이 존재하지 않습니다: rabbit.jpg")
	}

}
