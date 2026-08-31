package imagen

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agente-video/internal/proveedor"
)

// TestPollinationsEnVivo pega contra el servicio real. Se salta en modo corto:
//
//	go test ./internal/proveedor/imagen -run EnVivo -v
func TestPollinationsEnVivo(t *testing.T) {
	if testing.Short() {
		t.Skip("requiere red")
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelar()

	destino := filepath.Join(t.TempDir(), "prueba.png")
	p := NuevoPollinations("flux")

	escrita, err := p.Generar(ctx, proveedor.PeticionImagen{
		Prompt:  "a lonely lighthouse on a cliff at dusk, cinematic photography, moody lighting, 35mm film grain",
		Semilla: 1471,
		Ancho:   1080,
		Alto:    1920,
		Destino: destino,
	})
	if err != nil {
		t.Fatalf("Generar: %v", err)
	}

	info, err := os.Stat(escrita)
	if err != nil {
		t.Fatalf("no se escribió el archivo: %v", err)
	}
	if info.Size() < 10_000 {
		t.Fatalf("la imagen pesa solo %d bytes, parece inválida", info.Size())
	}

	// Verificamos la firma real del archivo, no solo que exista algo.
	cabecera := make([]byte, 8)
	f, err := os.Open(escrita)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Read(cabecera); err != nil {
		t.Fatal(err)
	}
	esPNG := string(cabecera[1:4]) == "PNG"
	esJPEG := cabecera[0] == 0xFF && cabecera[1] == 0xD8
	if !esPNG && !esJPEG {
		t.Fatalf("el archivo no es PNG ni JPEG: %x", cabecera)
	}

	t.Logf("imagen ok: %s, %d bytes, PNG=%v JPEG=%v", filepath.Ext(escrita), info.Size(), esPNG, esJPEG)
}
