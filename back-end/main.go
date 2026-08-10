package main

import (
	"back-end/auth"
	"back-end/filemanager"
	"back-end/models"
	"back-end/queue"
	"back-end/utils"

	"github.com/gin-gonic/gin"
)

func main() {
	db, err := utils.NewDB()
	if err != nil {
		panic("No fue posible conectarse a la base de datos: " + err.Error())
	}

	// Migrar el esquema de la base de datos
	db.AutoMigrate(&models.UserModel{}, &models.FileModel{}, &models.ArtifactModel{}, &models.ConversionModel{})

	// Crear el cliente de MinIO y garantizar que el bucket exista
	minioClient, err := utils.NewMinioClient()
	if err != nil {
		panic("No fue posible conectarse a MinIO: " + err.Error())
	}

	// Crear el enqueuer de conversiones en RabbitMQ
	enqueuer, err := queue.NewEnqueuer()
	if err != nil {
		panic("No fue posible conectarse a la cola de conversiones: " + err.Error())
	}
	defer enqueuer.Close()

	router := gin.Default()
	auth.SetupAuthRoutes(router, db)
	filemanager.SetupFileManagerRoutes(router, db, minioClient, enqueuer)
	router.Run(":8080")
}
