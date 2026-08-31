// Package imagen agrupa los generadores de imagen fija.
package imagen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// clienteHTTPTimeout es generoso: los generadores gratuitos encolan peticiones.
const clienteHTTPTimeout = 180 * time.Second

// escribirArchivo guarda los bytes usando la extensión que corresponde al
// contenido real, no a la que pidió quien llama: Pollinations devuelve JPEG
// aunque se le pida .png. Devuelve la ruta final escrita.
func escribirArchivo(destino string, datos []byte) (string, error) {
	if len(datos) == 0 {
		return "", fmt.Errorf("el proveedor devolvió una imagen vacía")
	}
	ext := detectarExtension(datos)
	if ext == "" {
		return "", fmt.Errorf("el proveedor no devolvió una imagen reconocible (%d bytes)", len(datos))
	}
	destino = strings.TrimSuffix(destino, filepath.Ext(destino)) + ext

	if err := os.MkdirAll(filepath.Dir(destino), 0o755); err != nil {
		return "", err
	}
	// Escritura atómica: si el proceso muere a media descarga, el checkpoint
	// no queda con un archivo truncado que parezca válido.
	tmp := destino + ".parcial"
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, destino); err != nil {
		return "", err
	}
	return destino, nil
}

func detectarExtension(d []byte) string {
	switch {
	case len(d) >= 8 && bytes.Equal(d[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return ".png"
	case len(d) >= 3 && d[0] == 0xFF && d[1] == 0xD8 && d[2] == 0xFF:
		return ".jpg"
	case len(d) >= 12 && bytes.Equal(d[:4], []byte("RIFF")) && bytes.Equal(d[8:12], []byte("WEBP")):
		return ".webp"
	default:
		return ""
	}
}
