package backend

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func AuthenticateUser(username, password string, context *gin.Context) bool {

}
