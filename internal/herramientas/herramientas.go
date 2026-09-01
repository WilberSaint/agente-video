// Package herramientas envuelve los binarios externos (ffmpeg, ffprobe, piper,
// whisper). Toda ejecución de procesos del agente pasa por aquí.
package herramientas

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// DirBin es la carpeta donde el agente busca binarios antes que en el PATH.
var DirBin = "bin"

// Buscar localiza un ejecutable: primero en DirBin, luego en el PATH.
// Devuelve un error explicando cómo instalarlo si no aparece.
func Buscar(nombre string) (string, error) {
	if runtime.GOOS == "windows" && !strings.HasSuffix(nombre, ".exe") {
		nombre += ".exe"
	}
	// Una ruta ya es una ruta: anteponerle DirBin daría bin/bin/... Se admite
	// para que un proveedor pueda apuntar a un intérprete concreto, como el
	// Python autocontenido, en vez de a un binario suelto de bin/.
	if strings.ContainsAny(nombre, `/\`) {
		if info, err := os.Stat(nombre); err == nil && !info.IsDir() {
			if abs, err := filepath.Abs(nombre); err == nil {
				return abs, nil
			}
			return nombre, nil
		}
		return "", fmt.Errorf("no se encontró %q", nombre)
	}

	local := filepath.Join(DirBin, nombre)
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		abs, err := filepath.Abs(local)
		if err == nil {
			return abs, nil
		}
		return local, nil
	}
	ruta, err := exec.LookPath(nombre)
	if err != nil {
		return "", fmt.Errorf("no se encontró %q ni en ./%s ni en el PATH; "+
			"ejecuta  agente-video instalar  para descargarlo", nombre, DirBin)
	}
	return ruta, nil
}

// Correr ejecuta un binario y devuelve stdout. En caso de fallo el error
// incluye las últimas líneas de stderr, que es donde ffmpeg explica todo.
func Correr(ctx context.Context, binario string, args ...string) (string, error) {
	return CorrerConEntrada(ctx, binario, "", args...)
}

func CorrerConEntrada(ctx context.Context, binario, stdin string, args ...string) (string, error) {
	ruta, err := Buscar(binario)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, ruta, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var salida, errores bytes.Buffer
	cmd.Stdout = &salida
	cmd.Stderr = &errores
	if err := cmd.Run(); err != nil {
		return salida.String(), fmt.Errorf("%s falló: %w\n%s",
			filepath.Base(ruta), err, ultimasLineas(errores.String(), 12))
	}
	return salida.String(), nil
}

// DuracionSeg devuelve la duración de un archivo de audio o video.
func DuracionSeg(ctx context.Context, archivo string) (float64, error) {
	salida, err := Correr(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		archivo)
	if err != nil {
		return 0, err
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(salida), 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe devolvió una duración ilegible (%q): %w", salida, err)
	}
	return d, nil
}

// RutaParaFiltro escapa una ruta de Windows para incrustarla dentro de un
// filtro de ffmpeg (subtitles=...), donde ':' y '\' tienen significado propio.
func RutaParaFiltro(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	abs = filepath.ToSlash(abs)
	abs = strings.ReplaceAll(abs, ":", `\:`)
	abs = strings.ReplaceAll(abs, "'", `\'`)
	return abs
}

func ultimasLineas(s string, n int) string {
	lineas := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lineas) > n {
		lineas = lineas[len(lineas)-n:]
	}
	return strings.Join(lineas, "\n")
}

// FuenteInstalada indica si Windows conoce una fuente por su nombre de familia.
//
// Importa porque libass, con el proveedor DirectWrite, ante un FontName que no
// existe no sustituye por otra: no dibuja nada. Los subtítulos desaparecen sin
// un solo mensaje de error y ffmpeg termina con código 0.
func FuenteInstalada(nombre string) bool {
	if runtime.GOOS != "windows" || strings.TrimSpace(nombre) == "" {
		return true // fuera de Windows no sabemos; no bloqueamos por eso
	}
	salida, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`).Output()
	if err != nil {
		return true // si no podemos consultar, damos el beneficio de la duda
	}
	return strings.Contains(strings.ToLower(string(salida)), strings.ToLower(nombre))
}
