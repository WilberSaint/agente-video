package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"agente-video/internal/proveedor"
)

func TestEscribirMetadatos(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")

	pl := &Pipeline{}
	pl.escribirMetadatos(video, &proveedor.GuionGenerado{
		Titulo:      "Por qué la constancia vence al talento",
		Descripcion: "Una idea corta para quien insiste en silencio.",
		Hashtags:    []string{"constancia", "disciplina"},
	})

	b, err := os.ReadFile(filepath.Join(dir, "clip.txt"))
	if err != nil {
		t.Fatalf("no se escribió el .txt: %v", err)
	}
	quiere := "TÍTULO\nPor qué la constancia vence al talento #viral\n\n" +
		"DESCRIPCIÓN\nUna idea corta para quien insiste en silencio.\n\n" +
		"#constancia #disciplina #viral\n"
	if string(b) != quiere {
		t.Errorf("el .txt quedó así:\n%s\nse esperaba:\n%s", b, quiere)
	}
}
