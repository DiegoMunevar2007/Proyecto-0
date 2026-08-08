package auth

import (
	"back-end/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func AuthenticateUser(username, password string, db *gorm.DB) bool {
	/*
		Autentica al usuario verificando su nombre de usuario y contraseña en la base de datos.
		Si la autenticación es exitosa, devuelve true; de lo contrario, devuelve false.
	*/
	var user models.UserModel
	result := db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		return false
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return false
	}
	return true
}

func GetUserID(username string, db *gorm.DB) (uint, error) {
	/*
		Obtiene el ID del usuario a partir de su nombre de usuario.
		Devuelve un error si el usuario no existe.
	*/
	var user models.UserModel
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		return 0, err
	}
	return user.ID, nil
}

func hashPassword(password string) string {
	/*
		Hace el hasheo de la contraseña utilizando bcrypt y devuelve la contraseña hasheada.
	*/
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(hashedPassword)
}

func RegisterUser(username, password string, db *gorm.DB) string {
	/*
		Registra un nuevo usuario en la base de datos con el nombre de usuario y la contraseña proporcionados.
		La contraseña se hashea antes de almacenarla en la base de datos.
	*/
	hashedPassword := hashPassword(password)
	user := models.UserModel{Username: username, Password: hashedPassword}
	result := db.Create(&user)
	if result.Error != nil {
		return result.Error.Error()
	}
	return "Usuario " + username + " registrado exitosamente"
}
