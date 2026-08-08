package auth

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupAuthRoutes(router *gin.Engine, db *gorm.DB) {
	/*
		Configura las rutas de autenticación en el enrutador Gin.
		Se definen las rutas para el registro y la autenticación de usuarios.
	*/
	authGroup := router.Group("/auth")
	{
		authGroup.POST("/register", func(c *gin.Context) {
			var request struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			//
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(400, gin.H{"error": "Solicitud inválida"})
				return
			}
			message := RegisterUser(request.Username, request.Password, db)
			c.JSON(200, gin.H{"message": message})
		})

		authGroup.POST("/login", func(c *gin.Context) {
			var request struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			if err := c.ShouldBindJSON(&request); err != nil {
				c.JSON(400, gin.H{"error": "Solicitud inválida"})
				return
			}
			if AuthenticateUser(request.Username, request.Password, db) {
				c.JSON(200, gin.H{"message": "Autenticación exitosa"})
			} else {
				c.JSON(401, gin.H{"error": "Nombre de usuario o contraseña incorrectos"})
			}
		})

		authGroup.GET("/me", RequireAuth(db), func(c *gin.Context) {
			c.JSON(200, gin.H{"username": c.GetString("username")})
		})
	}
}
