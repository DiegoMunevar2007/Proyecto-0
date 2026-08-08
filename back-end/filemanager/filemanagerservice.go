package filemanager

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"back-end/models"
	"back-end/utils"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

type UploadResult struct {
	FileID uint
	S3Key  string
}

func UploadFile(minioClient *minio.Client, bucketName string, filePath string, objectName string) (string, error) {
	// Subir el archivo al bucket de MinIO
	_, err := minioClient.FPutObject(context.Background(), bucketName, objectName, filePath, minio.PutObjectOptions{})
	if err != nil {
		return "", err
	}
	return objectName, nil
}

func HandleUpload(db *gorm.DB, minioClient *minio.Client, userID uint, fileHeader *multipart.FileHeader) (*UploadResult, error) {
	/*
		Guarda el archivo en el bucket de MinIO, crea el registro de metadatos
		en la tabla de archivos y la conversión pendiente. Devuelve la clave S3
		para que pueda ser guardada en las tablas de metadatos.
	*/
	s3Key := generateS3Key(fileHeader.Filename)

	// Guardar temporalmente el archivo para poder subirlo a MinIO
	tmp, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())

	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	if _, err := io.Copy(tmp, src); err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	if _, err := UploadFile(minioClient, utils.BucketName(), tmp.Name(), s3Key); err != nil {
		return nil, err
	}

	file := models.FileModel{
		UserID:      userID,
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		S3Key:       s3Key,
		Size:        fileHeader.Size,
	}
	if err := db.Create(&file).Error; err != nil {
		return nil, err
	}

	conversion := models.ConversionModel{
		FileID: file.ID,
		Status: models.ConversionPending,
	}
	if err := db.Create(&conversion).Error; err != nil {
		return nil, err
	}

	// TODO: Encolar la tarea de conversión en la cola de trabajadores (RabbitMQ).

	return &UploadResult{FileID: file.ID, S3Key: s3Key}, nil
}

func AllFilesUploadedByUser(db *gorm.DB, userID uint) ([]models.FileModel, error) {
	/*
		Obtiene todos los archivos subidos por un usuario a partir de su ID.
	*/
	var files []models.FileModel
	if err := db.Where("user_id = ?", userID).Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func DownloadFile(minioClient *minio.Client, bucketName string, s3Key string) (io.ReadCloser, error) {
	/*
		Descarga un archivo desde MinIO a partir de su clave S3.
	*/
	object, err := minioClient.GetObject(context.Background(), bucketName, s3Key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return object, nil
}

func generateS3Key(filename string) string {
	/*
		Genera una clave S3 única para un archivo subido, combinando un UUID con el nombre del archivo.
	*/
	return fmt.Sprintf("%s_%s", uuid.NewString(), filepath.Base(filename))
}
