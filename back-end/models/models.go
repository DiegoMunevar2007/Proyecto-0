package models

import (
	"time"

	"gorm.io/gorm"
)

type UserModel struct {
	/*
		Es el modelo para representar a un usuario en la base de datos.
		Contiene un nombre de usuario y una contraseña, además de la relación
		con los archivos que ha subido.
	*/
	gorm.Model

	Username string `gorm:"uniqueIndex;not null"`
	Password string `gorm:"not null"`

	Files []FileModel `gorm:"foreignKey:UserID"`
}

type FileModel struct {
	/*
		Es el modelo para representar un archivo subido por un usuario en la
		base de datos.

		Contiene el nombre del archivo, el tipo de contenido, la clave utilizada
		para almacenarlo en S3 y su tamaño. Además, contiene una relación con el
		usuario que subió el archivo y con la única conversión realizada sobre él.
	*/
	gorm.Model

	UserID uint      `gorm:"not null;index"`
	User   UserModel `gorm:"foreignKey:UserID"`

	Filename    string `gorm:"not null"`
	ContentType string `gorm:"not null"` // application/pdf, image/png, etc.
	S3Key       string `gorm:"uniqueIndex;not null"`
	Size        int64  `gorm:"not null"`

	// Conversión realizada sobre este archivo.
	Conversion *ConversionModel `gorm:"foreignKey:FileID"`
}

type ArtifactModel struct {
	/*
		Es el modelo para representar un artefacto de OKF generado a partir
		de un archivo subido por un usuario.

		Contiene el nombre del archivo, el tipo de contenido, la clave utilizada
		para almacenarlo en S3 y su tamaño. Además, contiene una relación con la
		conversión que generó este artefacto.
	*/
	gorm.Model

	Filename    string `gorm:"not null"`
	ContentType string `gorm:"not null"`
	S3Key       string `gorm:"uniqueIndex;not null"`
	Size        int64  `gorm:"not null"`

	// Conversión que generó este artefacto.
	Conversion *ConversionModel `gorm:"foreignKey:ArtifactID"`
}

type ConversionStatus string

const (
	// La conversión fue creada, pero aún no ha comenzado.
	ConversionPending ConversionStatus = "pendiente"

	// La conversión se encuentra actualmente en ejecución.
	ConversionProcessing ConversionStatus = "procesando"

	// La conversión terminó correctamente y generó un artefacto.
	ConversionCompleted ConversionStatus = "completada"

	// La conversión terminó con un error y no generó un artefacto.
	ConversionFailed ConversionStatus = "fallida"
)

type ConversionModel struct {
	/*
		Es el modelo que representa la conversión de un archivo a un artefacto.

		Relaciona el archivo de entrada con el artefacto generado y almacena
		el estado actual de la conversión. También contiene información sobre
		un posible error y las fechas de inicio y finalización del proceso.
	*/
	gorm.Model

	FileID uint      `gorm:"uniqueIndex;not null"`
	File   FileModel `gorm:"foreignKey:FileID"`

	// Es nulo mientras la conversión no haya generado un artefacto.
	ArtifactID *uint
	Artifact   *ArtifactModel `gorm:"foreignKey:ArtifactID"`

	Status ConversionStatus `gorm:"not null"`

	// Contiene el mensaje del error en caso de que la conversión falle.
	ErrorMessage *string

	// Fechas de inicio y finalización del proceso de conversión.
	StartedAt   *time.Time
	CompletedAt *time.Time
}
