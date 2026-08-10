package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func yamlQuote(value string) string {
	/*
		yamlQuote escapa un valor para que pueda ser usado como un string YAML de doble comilla.
		Escapa las comillas dobles y las barras invertidas, y envuelve el valor entre comillas dobles.
	*/
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return fmt.Sprintf("%q", escaped)
}

// accentReplacer normaliza caracteres acentuados para generar slugs seguros.
var accentReplacer = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"ü", "u", "ñ", "n", "à", "a", "è", "e", "ì", "i", "ò", "o", "ù", "u",
	"â", "a", "ê", "e", "î", "i", "ô", "o", "û", "u",
)

func slugify(title string) string {
	/*
		slugify genera un slug seguro para usar como nombre de archivo a partir
		de un título. Normaliza acentos, reemplaza caracteres no alfanuméricos por
		guiones y limita la longitud a 80 caracteres.
	*/
	slug := accentReplacer.Replace(strings.ToLower(title))
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-")
	}
	if slug == "" {
		slug = "documento"
	}
	return slug
}

// Heading representa un encabezado ATX del markdown. OriginalLevel es el
// número de '#' original y LogicalLevel el nivel normalizado (1 = raíz).
type Heading struct {
	OriginalLevel int
	LogicalLevel  int
	Title         string
}

// markdownSection es una unidad lógica del documento delimitada por los
// encabezados de nivel raíz. heading es nil solo si la sección contiene el
// texto introductorio anterior al primer encabezado.
type markdownSection struct {
	heading *Heading
	lines   []string
}

// headingRe reconoce encabezados ATX de nivel 1 a 6.
var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

func parseSections(markdown string) ([]markdownSection, bool) {
	/*
		parseSections analiza el markdown y lo divide en secciones según los
		encabezados de nivel raíz. Devuelve una lista de secciones y un booleano
		que indica si se encontraron encabezados.
	*/
	lines := strings.Split(markdown, "\n")

	// La raíz es el nivel de encabezado más superficial que se repite al
	// menos dos veces: es el nivel que estructura el documento. Un encabezado
	// único y poco profundo (p. ej. el título de un libro) no debe marcar la
	// raíz. Si ningún nivel se repite, se usa el nivel más superficial.
	counts := make(map[int]int)
	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			counts[len(m[1])]++
		}
	}
	rootLevel := 0
	for level := 1; level <= 6; level++ {
		if counts[level] >= 2 {
			rootLevel = level
			break
		}
	}
	if rootLevel == 0 {
		for level := 1; level <= 6; level++ {
			if counts[level] > 0 {
				rootLevel = level
				break
			}
		}
	}
	if rootLevel == 0 {
		return nil, false
	}

	var sections []markdownSection
	var intro []string
	var current *markdownSection
	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			if level == rootLevel {
				if current != nil {
					sections = append(sections, *current)
				}
				current = &markdownSection{
					heading: &Heading{
						OriginalLevel: level,
						LogicalLevel:  level - rootLevel + 1,
						Title:         strings.TrimSpace(m[2]),
					},
					lines: []string{line},
				}
				continue
			}
		}
		if current != nil {
			current.lines = append(current.lines, line)
		} else {
			intro = append(intro, line)
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}

	if len(intro) > 0 && len(sections) > 0 {
		sections[0].lines = append(intro, sections[0].lines...)
	}
	return sections, true
}

func uniqueSlug(base string, used map[string]bool) string {
	/*
		uniqueSlug genera un slug único basado en base. Si base ya está en used,
		agrega un sufijo numérico incremental hasta encontrar uno disponible.
	*/
	if used == nil {
		used = make(map[string]bool)
	}
	if !used[base] {
		used[base] = true
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

func extractDoclingZip(zipBytes []byte, destDir string) (mdPath string, artifactsPath string, err error) {
	/*
		extractDoclingZip extrae un ZIP generado por Docling Serve en destDir.
		Busca el archivo markdown principal y la carpeta de imágenes "artifacts/".
		Devuelve las rutas completas del markdown y de la carpeta de imágenes.
	*/
	mdPath = ""
	artifactsPath = ""
	reader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return "", "", fmt.Errorf("no fue posible leer el ZIP de Docling: %w", err)
	}
	for _, file := range reader.File {
		// Evitar rutas que escapen del directorio de extracción.
		name := filepath.FromSlash(file.Name)
		dest := filepath.Join(destDir, name)
		if !strings.HasPrefix(dest, filepath.Clean(destDir)+string(filepath.Separator)) {
			return "", "", fmt.Errorf("entrada del ZIP con ruta no segura: %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return "", "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return "", "", err
		}
		src, err := file.Open()
		if err != nil {
			return "", "", err
		}
		out, err := os.Create(dest)
		if err != nil {
			src.Close()
			return "", "", err
		}
		if _, err := io.Copy(out, src); err != nil {
			src.Close()
			out.Close()
			return "", "", err
		}
		src.Close()
		out.Close()

		rel := filepath.ToSlash(file.Name)
		switch {
		case strings.HasSuffix(rel, ".md") && !strings.Contains(rel, "/"):
			if mdPath == "" {
				mdPath = dest
			}
		case strings.HasPrefix(rel, "artifacts/"):
			artifactsPath = filepath.Join(destDir, "artifacts")
		}
	}
	if mdPath == "" {
		return "", "", fmt.Errorf("el ZIP de Docling no contiene un archivo markdown")
	}
	return mdPath, artifactsPath, nil
}

// buildOKFBundle crea un bundle OKF (Open Knowledge Format) a partir del ZIP
// generado por Docling Serve (markdown + imágenes en "artifacts/") y lo
// comprime en un archivo ZIP. La carpeta raíz del ZIP siempre se llama
// "bundle/" y contiene index.md, log.md, los conceptos (markdown de sección en
// la raíz) y las imágenes en "assets/". Devuelve la ruta del archivo ZIP
// generado. El llamador es responsable de eliminar el archivo.
func buildOKFBundle(zipBytes []byte, baseName string, resource string, now time.Time) (string, error) {
	safeBase := slugify(baseName)

	bundleDir, err := os.MkdirTemp("", "okf-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(bundleDir)

	// Extraer el ZIP de Docling a un directorio temporal aparte para que la
	// salida original no quede dentro del bundle final.
	doclingDir, err := os.MkdirTemp("", "okf-docling-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(doclingDir)

	mdPath, artifactsPath, err := extractDoclingZip(zipBytes, doclingDir)
	if err != nil {
		return "", err
	}
	markdown, err := os.ReadFile(mdPath)
	if err != nil {
		return "", err
	}

	// Mover las imágenes a "assets/" en la raíz del bundle.
	hasImages := false
	if artifactsPath != "" {
		if err := os.Rename(artifactsPath, filepath.Join(bundleDir, "assets")); err != nil {
			return "", err
		}
		hasImages = true
	}

	// rewriteRefs ajusta las referencias a imágenes. Las secciones y assets/
	// quedan en la raíz del bundle, por lo que el prefijo es siempre "assets/".
	rewriteRefs := func(content string) string {
		if !hasImages {
			return content
		}
		return strings.ReplaceAll(content, "artifacts/", "assets/")
	}

	raw := string(markdown)
	var entries []string
	sections, hasHeadings := parseSections(rewriteRefs(raw))
	if !hasHeadings {
		// Sin encabezados: un solo concepto con todo el documento.
		name := safeBase + ".md"
		content := frontmatter(map[string]string{
			"type":        "Document",
			"title":       baseName,
			"description": "Documento convertido",
			"resource":    resource,
			"timestamp":   now.Format(time.RFC3339),
		}) + "\n" + strings.TrimSpace(rewriteRefs(raw)) + "\n"
		if err := os.WriteFile(filepath.Join(bundleDir, name), []byte(content), 0o644); err != nil {
			return "", err
		}
		entries = append(entries, name)
	} else {
		usedSlugs := make(map[string]bool)
		for _, section := range sections {
			name := uniqueSlug(slugify(section.heading.Title), usedSlugs) + ".md"
			content := frontmatter(map[string]string{
				"type":        "Section",
				"title":       section.heading.Title,
				"description": "Sección del documento convertido",
				"resource":    resource,
				"tags":        "[documento]",
				"timestamp":   now.Format(time.RFC3339),
			}) + "\n" + strings.TrimSpace(strings.Join(section.lines, "\n")) + "\n"
			if err := os.WriteFile(filepath.Join(bundleDir, name), []byte(content), 0o644); err != nil {
				return "", err
			}
			entries = append(entries, name)
		}
	}

	// index.md: lista los conceptos del bundle.
	indexLines := []string{
		"---",
		"okf_version: \"0.1\"",
		"---",
		"",
		"# " + baseName,
		"",
	}
	for _, entry := range entries {
		indexLines = append(indexLines, fmt.Sprintf("- [%s](%s)", entry, entry))
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "index.md"), []byte(strings.Join(indexLines, "\n")+"\n"), 0o644); err != nil {
		return "", err
	}

	// log.md: historial de cambios del bundle.
	logContent := "# Bitácora del bundle\n\n## " + now.Format("2006-01-02") + "\n- **Creación**: se generó el bundle a partir de `" + resource + "`.\n"
	if err := os.WriteFile(filepath.Join(bundleDir, "log.md"), []byte(logContent), 0o644); err != nil {
		return "", err
	}

	// Comprimir el directorio del bundle en un archivo ZIP temporal.
	zipFile, err := os.CreateTemp("", safeBase+"-*.okf.zip")
	if err != nil {
		return "", err
	}
	zipPath := zipFile.Name()

	writer := zip.NewWriter(zipFile)
	if err := addDirToZip(writer, bundleDir, "bundle"); err != nil {
		zipFile.Close()
		os.Remove(zipPath)
		return "", err
	}
	if err := writer.Close(); err != nil {
		zipFile.Close()
		os.Remove(zipPath)
		return "", err
	}
	if err := zipFile.Close(); err != nil {
		os.Remove(zipPath)
		return "", err
	}

	return zipPath, nil
}

// frontmatter genera el bloque YAML de encabezado de un concepto OKF.
func frontmatter(fields map[string]string) string {
	var builder strings.Builder
	builder.WriteString("---\n")
	for key, value := range fields {
		builder.WriteString(fmt.Sprintf("%s: %s\n", key, yamlQuote(value)))
	}
	builder.WriteString("---\n")
	return builder.String()
}

// addDirToZip agrega recursivamente el contenido de dir al ZIP bajo el prefijo.
func addDirToZip(writer *zip.Writer, dir string, prefix string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath := strings.TrimPrefix(path, dir)
		relPath = strings.TrimPrefix(relPath, string(filepath.Separator))
		if prefix != "" {
			relPath = filepath.Join(prefix, relPath)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = entry.Write(content)
		return err
	})
}
