package filemanager

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	"back-end/auth"
	"back-end/queue"
	"back-end/utils"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"gorm.io/gorm"
)

type fileManagerHandler struct {
	db          *gorm.DB
	minioClient *minio.Client
	publisher   *queue.Enqueuer
}

func createFileManagerHandler(db *gorm.DB, minioClient *minio.Client, publisher *queue.Enqueuer) *fileManagerHandler {
	return &fileManagerHandler{db: db, minioClient: minioClient, publisher: publisher}
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

	result, err := HandleUpload(h.db, h.minioClient, h.publisher, userID, fileHeader)
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
		Maneja la descarga de un archivo desde MinIO a partir de su clave S3.
		Verifica que el usuario esté autenticado, obtiene la clave S3 del archivo
		y llama a la función DownloadFile para obtener el archivo desde MinIO.
	*/
	s3Key := c.Param("s3Key")
	if s3Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Se requiere la clave S3 del archivo"})
		return
	}

	h.streamObject(c, s3Key, s3Key, "")
}

func (h *fileManagerHandler) downloadOriginalFile(c *gin.Context) {
	/*
		Maneja la descarga del archivo original de un usuario a partir del ID
		del archivo, verificando que le pertenezca al usuario autenticado.
	*/
	userID, err := auth.GetUserID(c.GetString("username"), h.db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no encontrado"})
		return
	}

	fileID, err := parseFileID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Se requiere un ID de archivo válido"})
		return
	}

	file, err := FindFileOwnedByUser(h.db, userID, fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No fue posible encontrar el archivo"})
		return
	}

	h.streamObject(c, file.S3Key, file.Filename, file.ContentType)
}

func (h *fileManagerHandler) downloadArtifactFile(c *gin.Context) {
	/*
		Maneja la descarga del artefacto generado a partir del archivo de un
		usuario, buscándolo por el ID del archivo y verificando que le
		pertenezca al usuario autenticado.
	*/
	userID, err := auth.GetUserID(c.GetString("username"), h.db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Usuario no encontrado"})
		return
	}

	fileID, err := parseFileID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Se requiere un ID de archivo válido"})
		return
	}

	file, err := FindFileOwnedByUser(h.db, userID, fileID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No fue posible encontrar el archivo"})
		return
	}

	artifact := file.Conversion.Artifact
	if artifact == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "La conversión aún no ha generado un artefacto"})
		return
	}

	h.streamObject(c, artifact.S3Key, artifact.Filename, artifact.ContentType)
}

// parseFileID convierte el parámetro :fileId de la ruta a un uint.
func parseFileID(c *gin.Context) (uint, error) {
	fileID, err := strconv.ParseUint(c.Param("fileId"), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(fileID), nil
}

func (h *fileManagerHandler) streamObject(c *gin.Context, s3Key string, filename string, contentType string) {
	/*
		streamObject descarga un archivo desde MinIO y lo envía como respuesta HTTP.
		Recibe la clave S3 del archivo, el nombre del archivo y el tipo de contenido.
	*/
	file, err := DownloadFile(h.minioClient, utils.BucketName(), s3Key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No fue posible descargar el archivo"})
		return
	}
	defer file.Close()

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Disposition", "attachment; filename="+url.PathEscape(filename))
	c.Header("Content-Type", contentType)
	c.Stream(func(w io.Writer) bool {
		io.Copy(w, file)
		return false
	})
}

func SetupFileManagerRoutes(router *gin.Engine, db *gorm.DB, minioClient *minio.Client, publisher *queue.Enqueuer) {
	/*
		Configura las rutas de manejo de archivos en el enrutador Gin.
		Todas las rutas requieren autenticación.
	*/
	handler := createFileManagerHandler(db, minioClient, publisher)

	filesGroup := router.Group("/files")
	{
		filesGroup.POST("/upload", auth.RequireAuth(db), handler.uploadFile)
		filesGroup.GET("/uploaded", auth.RequireAuth(db), handler.viewAllFilesUploadedByUser)
		filesGroup.GET("/download/:s3Key", auth.RequireAuth(db), handler.downloadFile)
		filesGroup.GET("/:fileId/original", auth.RequireAuth(db), handler.downloadOriginalFile)
		filesGroup.GET("/:fileId/artifact", auth.RequireAuth(db), handler.downloadArtifactFile)
	}
}
