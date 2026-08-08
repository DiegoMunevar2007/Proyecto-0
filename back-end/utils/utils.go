package utils

import (
	"context"
	"fmt"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func GetEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func BucketName() string {
	return GetEnv("MINIO_BUCKET", "uploads")
}

func NewDB() (*gorm.DB, error) {
	/*
		Crea la conexión a la base de datos PostgreSQL a partir de las
		variables de entorno. Si no están definidas, se utilizan los valores
		por defecto del docker-compose.
	*/
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		GetEnv("DB_HOST", "localhost"),
		GetEnv("POSTGRES_USER", "cloudUser"),
		GetEnv("POSTGRES_PASSWORD", "cloudPassword"),
		GetEnv("POSTGRES_DB", "clouddb"),
		GetEnv("DB_PORT", "5432"),
	)
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func NewMinioClient() (*minio.Client, error) {
	/*
		Crea un cliente de MinIO a partir de las variables de entorno.
		Si no están definidas, se utilizan los valores por defecto del
		docker-compose. También garantiza que el bucket exista.
	*/
	endpoint := GetEnv("MINIO_ENDPOINT", "localhost:9000")
	accessKey := GetEnv("MINIO_ACCESS_KEY", "minioadmin")
	secretKey := GetEnv("MINIO_SECRET_KEY", "minioadmin")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, BucketName())
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, BucketName(), minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}

	return client, nil
}
