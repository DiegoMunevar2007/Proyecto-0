package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"back-end/utils"

	"resty.dev/v3"
)

func requestConversionZip(File []byte, FileName string, ConversionType string) ([]byte, error) {
	/*
		requestConversionZip envía un archivo a Docling Serve para convertirlo a Markdown y obtener un ZIP con el resultado.
		Recibe el contenido del archivo, su nombre y el tipo de conversión (por ejemplo, "md" para Markdown).
		Devuelve el contenido del ZIP resultante o un error si la conversión falla.
	*/
	host := utils.GetEnv("DOCLING_HOST", "localhost")
	port := utils.GetEnv("DOCLING_PORT", "5001")
	url := fmt.Sprintf("http://%s:%s/v1/convert/file", host, port)

	toFormat := ConversionType
	if toFormat == "" {
		toFormat = "md"
	}

	client := resty.New()
	resp, err := client.R().
		SetFileReader("files", FileName, bytes.NewReader(File)).
		SetFormData(map[string]string{
			"to_formats":        toFormat,
			"image_export_mode": "referenced",
			"include_images":    "true",
			"images_scale":      "2.0",
			"target_type":       "zip",
		}).
		Post(url)

	if err != nil {
		return nil, err
	}
	// Verificar el código de estado HTTP de la respuesta.
	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return nil, fmt.Errorf("Docling Serve respondió con estado %d: %s", resp.StatusCode(), resp.String())
	}
	// Verificar que la respuesta sea el ZIP con el markdown y las imágenes.
	contentType := resp.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/zip") {
		return nil, fmt.Errorf("Docling Serve respondió con tipo %q en lugar de application/zip", contentType)
	}
	// Leer el cuerpo de la respuesta (el archivo ZIP).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("No fue posible leer la respuesta de Docling Serve: %w", err)
	}

	return body, nil
}
