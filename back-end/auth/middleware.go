package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RequireAuth(db *gorm.DB) gin.HandlerFunc {
	/*
		Middleware que autentica una solicitud leyendo las credenciales de usuario.
		Si la solicitud es multipart/form-data, las credenciales se leen de los
		campos del formulario; de lo contrario, desde el cuerpo JSON ({username, password}).
		Si son válidas, guarda el username en el contexto de gin; si no, responde 401.
	*/
	return func(c *gin.Context) {
		var request struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			request.Username = c.PostForm("username")
			request.Password = c.PostForm("password")
		} else if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Credenciales requeridas"})
			c.Abort()
			return
		}
		if !AuthenticateUser(request.Username, request.Password, db) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Nombre de usuario o contraseña incorrectos"})
			c.Abort()
			return
		}
		c.Set("username", request.Username)
		c.Next()
	}
}
