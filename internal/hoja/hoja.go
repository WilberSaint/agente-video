// Package hoja parte una hoja de expresiones en imágenes sueltas.
//
// Una hoja —varios retratos del mismo personaje en una rejilla— es la mejor
// forma de conseguir expresiones coherentes: al salir todas de la misma
// generación, el personaje es idéntico. Generándolas por separado siempre hay
// deriva, que es el mismo problema de coherencia que aparece entre planos.
//
// El corte se hace detectando las bandas vacías, no dividiendo en una rejilla
// fija: las hojas reales rara vez tienen el mismo número de paneles por fila, y
// una división aritmética partiría los personajes por la mitad.
package hoja

import (
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
)

type Opciones struct {
	// UmbralClaro es a partir de qué luminancia (0-255) se considera fondo.
	UmbralClaro uint8
	// MinOcupacion es qué fracción de píxeles no-fondo debe tener una fila o
	// columna para contar como contenido. Un valor pequeño evita que una mota
	// suelta una dos paneles.
	MinOcupacion float64
	// MinLado descarta recortes diminutos: restos de borde, no paneles.
	MinLado int
	// Margen recorta unos píxeles de cada panel, para que no se cuele la línea
	// del marco cuando la hoja los lleva dibujados.
	Margen int
}

func PorDefecto() Opciones {
	return Opciones{UmbralClaro: 232, MinOcupacion: 0.012, MinLado: 60, Margen: 4}
}

type Panel struct {
	Rect   image.Rectangle
	Fila   int
	Indice int
}

// Partir detecta los paneles y los escribe numerados en destino.
func Partir(origen, destino string, opt Opciones) ([]string, error) {
	f, err := os.Open(origen)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", origen, err)
	}

	paneles := Detectar(img, opt)
	if len(paneles) == 0 {
		return nil, fmt.Errorf("no se detectó ningún panel en %s; puede que el "+
			"fondo no sea claro o que los paneles se toquen entre sí", origen)
	}
	if err := os.MkdirAll(destino, 0o755); err != nil {
		return nil, err
	}

	var escritos []string
	for i, p := range paneles {
		recorte := recortar(img, p.Rect)
		ruta := filepath.Join(destino, fmt.Sprintf("%02d.png", i+1))
		out, err := os.Create(ruta)
		if err != nil {
			return escritos, err
		}
		if err := png.Encode(out, recorte); err != nil {
			out.Close()
			return escritos, err
		}
		out.Close()
		escritos = append(escritos, ruta)
	}
	return escritos, nil
}

// Detectar encuentra los paneles: primero las bandas horizontales con
// contenido, y dentro de cada una las columnas con contenido.
func Detectar(img image.Image, opt Opciones) []Panel {
	b := img.Bounds()
	ancho, alto := b.Dx(), b.Dy()
	if ancho == 0 || alto == 0 {
		return nil
	}

	ocupadoFila := make([]bool, alto)
	for y := 0; y < alto; y++ {
		n := 0
		for x := 0; x < ancho; x++ {
			if esContenido(img.At(b.Min.X+x, b.Min.Y+y), opt.UmbralClaro) {
				n++
			}
		}
		ocupadoFila[y] = float64(n)/float64(ancho) >= opt.MinOcupacion
	}

	var paneles []Panel
	for iFila, banda := range tramosVerdaderos(ocupadoFila) {
		if banda.fin-banda.ini < opt.MinLado {
			continue
		}
		ocupadoCol := make([]bool, ancho)
		altoBanda := banda.fin - banda.ini
		for x := 0; x < ancho; x++ {
			n := 0
			for y := banda.ini; y < banda.fin; y++ {
				if esContenido(img.At(b.Min.X+x, b.Min.Y+y), opt.UmbralClaro) {
					n++
				}
			}
			ocupadoCol[x] = float64(n)/float64(altoBanda) >= opt.MinOcupacion
		}

		for _, col := range tramosVerdaderos(ocupadoCol) {
			if col.fin-col.ini < opt.MinLado {
				continue
			}
			r := image.Rect(
				b.Min.X+col.ini+opt.Margen, b.Min.Y+banda.ini+opt.Margen,
				b.Min.X+col.fin-opt.Margen, b.Min.Y+banda.fin-opt.Margen,
			)
			if r.Dx() < opt.MinLado || r.Dy() < opt.MinLado {
				continue
			}
			paneles = append(paneles, Panel{Rect: r, Fila: iFila, Indice: len(paneles)})
		}
	}
	return paneles
}

type tramo struct{ ini, fin int }

// tramosVerdaderos devuelve los intervalos contiguos de valores true.
func tramosVerdaderos(v []bool) []tramo {
	var out []tramo
	ini := -1
	for i, ok := range v {
		if ok && ini < 0 {
			ini = i
		}
		if !ok && ini >= 0 {
			out = append(out, tramo{ini, i})
			ini = -1
		}
	}
	if ini >= 0 {
		out = append(out, tramo{ini, len(v)})
	}
	return out
}

// esContenido decide si un píxel es dibujo o fondo. Se usa luminancia y no el
// color exacto para que funcione igual con fondos blancos, crema o gris claro.
func esContenido(c color.Color, umbral uint8) bool {
	r, g, b, a := c.RGBA()
	if a < 0x8000 {
		return false // transparente cuenta como fondo
	}
	// Coeficientes de luminancia perceptual.
	lum := (299*int(r>>8) + 587*int(g>>8) + 114*int(b>>8)) / 1000
	return lum < int(umbral)
}

func recortar(img image.Image, r image.Rectangle) image.Image {
	salida := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			salida.Set(x, y, img.At(r.Min.X+x, r.Min.Y+y))
		}
	}
	return salida
}
