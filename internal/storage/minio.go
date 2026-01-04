package storage

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioClient struct {
	client  *minio.Client
	bucket  string
	enabled bool
}

func NewMinioFromEnv() (*MinioClient, error) {
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("MINIO_ENABLED")), "true")
	if !enabled {
		return &MinioClient{enabled: false}, nil
	}

	endpoint := strings.TrimSpace(os.Getenv("MINIO_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("MINIO_ROOT_USER"))
	secretKey := strings.TrimSpace(os.Getenv("MINIO_ROOT_PASSWORD"))
	bucket := strings.TrimSpace(os.Getenv("MINIO_BUCKET"))

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("minio config missing (endpoint, user, password, bucket)")
	}

	useSSL := strings.EqualFold(strings.TrimSpace(os.Getenv("MINIO_USE_SSL")), "true")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("init minio client: %w", err)
	}

	return &MinioClient{
		client:  client,
		bucket:  bucket,
		enabled: true,
	}, nil
}

func (m *MinioClient) Enabled() bool {
	return m != nil && m.enabled
}

func (m *MinioClient) Bucket() string {
	if m == nil {
		return ""
	}
	return m.bucket
}

func (m *MinioClient) UploadFile(ctx context.Context, objectKey, filePath, contentType string) (string, int64, error) {
	if !m.Enabled() {
		return "", 0, fmt.Errorf("minio disabled")
	}
	if contentType == "" {
		contentType = detectContentType(filePath)
	}

	info, err := m.client.FPutObject(ctx, m.bucket, objectKey, filePath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", 0, err
	}
	return info.ETag, info.Size, nil
}

func (m *MinioClient) UploadBytes(ctx context.Context, objectKey string, data []byte, contentType string) (string, int64, error) {
	if !m.Enabled() {
		return "", 0, fmt.Errorf("minio disabled")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	reader := bytes.NewReader(data)
	info, err := m.client.PutObject(ctx, m.bucket, objectKey, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", 0, err
	}
	return info.ETag, info.Size, nil
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "application/octet-stream"
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func SafeObjectKey(parts ...string) string {
	safeParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		part = strings.ReplaceAll(part, "\\", "/")
		part = strings.Trim(part, "/")
		part = strings.ReplaceAll(part, " ", "_")
		if part != "" {
			safeParts = append(safeParts, part)
		}
	}
	return strings.Join(safeParts, "/")
}
