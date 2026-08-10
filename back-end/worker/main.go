package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"back-end/filemanager"
	"back-end/models"
	"back-end/queue"
	"back-end/utils"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/wagslane/go-rabbitmq"
	"gorm.io/gorm"
)

func main() {
	// Conexión a la base de datos compartida con la API.
	db, err := utils.NewDB()
	if err != nil {
		log.Fatalf("No fue posible conectarse a la base de datos: %v", err)
	}

	// Verificar que los modelos compartidos funcionan.
	var conversionCount int64
	if err := db.Model(&models.ConversionModel{}).Count(&conversionCount).Error; err != nil {
		log.Fatalf("No fue posible consultar la base de datos: %v", err)
	}
	log.Printf("Workers conectado a la base de datos, conversiones pendientes: %d", conversionCount)

	// Conexión al almacenamiento de objetos compartido con la API.
	minioClient, err := utils.NewMinioClient()
	if err != nil {
		log.Fatalf("No fue posible conectarse a MinIO: %v", err)
	}
	log.Println("Worker conectado a MinIO")

	// Conexión a la cola de conversiones y consumo de trabajos. El worker
	// declara la cola y el binding al arrancar (idempotente).
	conn, err := queue.NewConn()
	if err != nil {
		log.Fatalf("No fue posible conectarse a la cola de conversiones: %v", err)
	}
	defer conn.Close()

	consumer, err := queue.NewConsumer(conn)
	if err != nil {
		log.Fatalf("No fue posible declarar la topología de la cola: %v", err)
	}
	defer consumer.Close()

	// Consumir los trabajos de forma asíncrona.
	go func() {
		if err := consumer.Run(func(delivery rabbitmq.Delivery) rabbitmq.Action {
			return handleConversionJob(db, minioClient, delivery)
		}); err != nil {
			log.Fatalf("Consumidor de conversiones detenido: %v", err)
		}
	}()

	log.Println("Worker iniciado")
	select {}
}

func handleConversionJob(db *gorm.DB, minioClient *minio.Client, delivery rabbitmq.Delivery) rabbitmq.Action {
	/*
		handleConversionJob procesa un mensaje de la cola de conversiones.
		El mensaje contiene el ID de la conversión que debe procesarse. El worker
		consulta los metadatos de la conversión en la base de datos, obtiene el
		archivo correspondiente desde S3, lo envía a Docling Serve para convertirlo
		a Markdown y generar un bundle OKF, y finalmente sube el artefacto a S3 y
		actualiza el estado de la conversión en la base de datos.
	*/
	var job queue.ConversionJob
	if err := json.Unmarshal(delivery.Body, &job); err != nil {
		log.Printf("Mensaje inválido en la cola de conversiones: %v", err)
		return rabbitmq.NackDiscard
	}
	log.Printf("Conversión recibida: %d", job.ConversionID)
	ConversionModel := models.ConversionModel{}
	if err := db.Preload("File").First(&ConversionModel, job.ConversionID).Error; err != nil {
		log.Printf("No fue posible consultar la conversión %d: %v", job.ConversionID, err)
		return rabbitmq.NackDiscard
	}
	log.Printf("Conversión encontrada: %d, estado: %s", ConversionModel.ID, ConversionModel.Status)

	startedAt := time.Now()
	if err := db.Model(&ConversionModel).Updates(map[string]interface{}{
		"status":     models.ConversionProcessing,
		"started_at": startedAt,
	}).Error; err != nil {
		log.Printf("No fue posible marcar la conversión %d como procesando: %v", job.ConversionID, err)
		return rabbitmq.NackDiscard
	}

	fileName := ConversionModel.File.Filename
	S3OriginalKey := ConversionModel.File.S3Key

	original, err := filemanager.DownloadFile(minioClient, utils.BucketName(), S3OriginalKey)
	if err != nil {
		log.Printf("No fue posible descargar el archivo original %s: %v", S3OriginalKey, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}
	defer original.Close()
	fileContent, err := io.ReadAll(original)
	if err != nil {
		log.Printf("No fue posible leer el archivo original %s: %v", S3OriginalKey, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}
	log.Printf("Archivo original descargado: %s (%d bytes)", S3OriginalKey, len(fileContent))

	// Esperar la conversión del archivo en Docling Serve.
	convertedZip, err := requestConversionZip(fileContent, fileName, "")
	if err != nil {
		log.Printf("No fue posible convertir el archivo %s: %v", fileName, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}
	log.Printf("Archivo %s convertido: %d bytes de ZIP con markdown e imágenes", fileName, len(convertedZip))

	// Procesar el resultado en un bundle OKF y comprimirlo.
	baseName := strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
	zipPath, err := buildOKFBundle(convertedZip, baseName, S3OriginalKey, startedAt)
	if err != nil {
		log.Printf("No fue posible generar el bundle OKF para %s: %v", fileName, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}
	defer os.Remove(zipPath)

	// Subir el bundle OKF a MinIO como artefacto.
	artifactName := slugify(baseName) + ".okf.zip"
	artifactKey := fmt.Sprintf("%s_%s", uuid.NewString(), artifactName)
	if _, err := filemanager.UploadFile(minioClient, utils.BucketName(), zipPath, artifactKey); err != nil {
		log.Printf("No fue posible subir el artefacto %s: %v", artifactKey, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}
	log.Printf("Artefacto subido a MinIO: %s", artifactKey)

	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		log.Printf("No fue posible consultar el artefacto %s: %v", artifactKey, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}

	// Registrar el artefacto y completar la conversión de forma atómica.
	if err := db.Transaction(func(tx *gorm.DB) error {
		artifact := models.ArtifactModel{
			Filename:    artifactName,
			ContentType: "application/zip",
			S3Key:       artifactKey,
			Size:        zipInfo.Size(),
		}
		if err := tx.Create(&artifact).Error; err != nil {
			return err
		}
		return tx.Model(&ConversionModel).Updates(map[string]interface{}{
			"artifact_id":  artifact.ID,
			"status":       models.ConversionCompleted,
			"completed_at": time.Now(),
		}).Error
	}); err != nil {
		log.Printf("No fue posible guardar el artefacto y completar la conversión %d: %v", job.ConversionID, err)
		markConversionFailed(db, &ConversionModel, err)
		return rabbitmq.Ack
	}
	log.Printf("Conversión %d completada, artefacto %s", job.ConversionID, artifactName)

	return rabbitmq.Ack
}

func markConversionFailed(db *gorm.DB, conversion *models.ConversionModel, err error) {
	/*
		markConversionFailed registra el error y marca la conversión como fallida en la base de datos.
		Se utiliza cuando ocurre un error durante el procesamiento de la conversión.
	*/
	errorMessage := err.Error()
	updateErr := db.Model(conversion).Updates(map[string]interface{}{
		"status":        models.ConversionFailed,
		"error_message": errorMessage,
		"completed_at":  time.Now(),
	}).Error
	if updateErr != nil {
		log.Printf("No fue posible marcar la conversión %d como fallida: %v", conversion.ID, updateErr)
	}
}
