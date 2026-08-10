package filemanager

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"

	"back-end/models"
	"back-end/queue"
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

func HandleUpload(db *gorm.DB, minioClient *minio.Client, publisher *queue.Enqueuer, userID uint, fileHeader *multipart.FileHeader) (*UploadResult, error) {
	/*
		Guarda el archivo en el bucket de MinIO, crea el registro de metadatos
		en la tabla de archivos y la conversión pendiente, y encola el trabajo
		en la cola de conversiones. Devuelve la clave S3 para que pueda ser
		guardada en las tablas de metadatos.
	*/
	s3Key := generateS3Key(fileHeader.Filename)

	// Guardar temporalmente el archivo para poder subirlo a MinIO
	tmp, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	// Abrir el archivo subido y copiar su contenido al archivo temporal
	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	// Copiar el contenido del archivo subido al archivo temporal
	if _, err := io.Copy(tmp, src); err != nil {
		return nil, err
	}
	// Cerrar el archivo temporal antes de subirlo a MinIO
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	if _, err := UploadFile(minioClient, utils.BucketName(), tmp.Name(), s3Key); err != nil {
		return nil, err
	}

	// Crear el archivo y la conversión pendiente de forma atómica.
	var file models.FileModel
	var conversion models.ConversionModel
	if err := db.Transaction(func(tx *gorm.DB) error {
		file = models.FileModel{
			UserID:      userID,
			Filename:    fileHeader.Filename,
			ContentType: fileHeader.Header.Get("Content-Type"),
			S3Key:       s3Key,
			Size:        fileHeader.Size,
		}
		if err := tx.Create(&file).Error; err != nil {
			return err
		}

		conversion = models.ConversionModel{
			FileID: file.ID,
			Status: models.ConversionPending,
		}
		return tx.Create(&conversion).Error
	}); err != nil {
		return nil, err
	}

	// Encolar el trabajo con el ID de la conversión para que el worker la procese.
	if err := publisher.EnqueueConversion(conversion.ID); err != nil {
		// Si el encolado falla, marcar la conversión como fallida para mantener
		// el estado consistente.
		errorMessage := "No fue posible encolar la conversión: " + err.Error()
		db.Model(&conversion).Updates(map[string]interface{}{
			"status":        models.ConversionFailed,
			"error_message": errorMessage,
		})
		return nil, err
	}

	return &UploadResult{FileID: file.ID, S3Key: s3Key}, nil
}

func AllFilesUploadedByUser(db *gorm.DB, userID uint) ([]models.FileModel, error) {
	/*
		Obtiene todos los archivos subidos por un usuario a partir de su ID.
	*/
	var files []models.FileModel
	if err := db.Preload("Conversion").Preload("Conversion.Artifact").Where("user_id = ?", userID).Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func FindFileOwnedByUser(db *gorm.DB, userID uint, fileID uint) (*models.FileModel, error) {
	/*
		Obtiene el archivo de un usuario junto con su conversión y artefacto,
		verificando que el archivo le pertenezca al usuario.
	*/
	var file models.FileModel
	if err := db.Preload("Conversion").Preload("Conversion.Artifact").
		First(&file, "id = ? AND user_id = ?", fileID, userID).Error; err != nil {
		return nil, err
	}
	return &file, nil
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
