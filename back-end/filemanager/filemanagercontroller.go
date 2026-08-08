package filemanager

import (
	"io"
	"net/http"

	"back-end/auth"
	"back-end/utils"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

type fileManagerHandler struct {
	db          *gorm.DB
	minioClient *minio.Client
}

func createFileManagerHandler(db *gorm.DB, minioClient *minio.Client) *fileManagerHandler {
	return &fileManagerHandler{db: db, minioClient: minioClient}
}

func (h *fileManagerHandler) uploadFile(c *gin.Context) {
	/*
		Maneja la subida de un archivo por parte de un usuario autenticado.
		Verifica que el usuario esté autenticado, obtiene el archivo del formulario
		multipart y llama a la función HandleUpload para procesarlo.
	*/

	// Obtener el ID del usuario autenticado.
	userID, err := auth.GetUserID(c.GetString("username"), h.db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no encontrado"})
		return
	}

	// Obtener el archivo subido desde el formulario multipart.
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Se requiere un archivo en el campo 'file'"})
		return
	}

	result, err := HandleUpload(h.db, h.minioClient, userID, fileHeader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No fue posible subir el archivo"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Archivo subido exitosamente",
		"file_id": result.FileID,
		"s3_key":  result.S3Key,
	})
}

func (h *fileManagerHandler) viewAllFilesUploadedByUser(c *gin.Context) {
	/*
		Obtiene todos los archivos subidos por un usuario específico.
		Devuelve una lista de modelos de archivo y un error si ocurre algún problema.
	*/
	userID, err := auth.GetUserID(c.GetString("username"), h.db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no encontrado"})
		return
	}
	files, err := AllFilesUploadedByUser(h.db, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No fue posible obtener los archivos"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

func (h *fileManagerHandler) downloadFile(c *gin.Context) {
	/*
		Maneja la descarga de un archivo desde MinIO.
		Verifica que el usuario esté autenticado, obtiene la clave S3 del archivo
		y llama a la función DownloadFile para obtener el archivo desde MinIO.
	*/
	s3Key := c.Param("s3Key")
	if s3Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Se requiere la clave S3 del archivo"})
		return
	}

	file, err := DownloadFile(h.minioClient, utils.BucketName(), s3Key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No fue posible descargar el archivo"})
		return
	}
	defer file.Close()

	c.Header("Content-Disposition", "attachment; filename="+s3Key)
	c.Header("Content-Type", "application/octet-stream")
	c.Stream(func(w io.Writer) bool {
		io.Copy(w, file)
		return false
	})
}

func SetupFileManagerRoutes(router *gin.Engine, db *gorm.DB, minioClient *minio.Client) {
	/*
		Configura las rutas de manejo de archivos en el enrutador Gin.
		Todas las rutas requieren autenticación.
	*/
	handler := createFileManagerHandler(db, minioClient)

	filesGroup := router.Group("/files")
	{
		filesGroup.POST("/upload", auth.RequireAuth(db), handler.uploadFile)
		filesGroup.GET("/uploaded", auth.RequireAuth(db), handler.viewAllFilesUploadedByUser)
		filesGroup.GET("/download/:s3Key", auth.RequireAuth(db), handler.downloadFile)
	}
}
