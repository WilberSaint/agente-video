package video

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

// Un personaje con varias expresiones.
//
// Una sola imagen fija durante cincuenta segundos deja de leerse como personaje
// y pasa a leerse como marca de agua: el ojo la descarta a los pocos segundos.
// Cambiándola al ritmo de las escenas, el personaje parece reaccionar a lo que
// se cuenta, y eso es justo lo que sostiene la atención.
//
// No hace falta que las expresiones encajen con el contenido —el agente no sabe
// si una escena es alegre o tensa—; basta con que cambien.

// expresionesDe devuelve las imágenes del personaje. Si Imagen apunta a una
// carpeta, se toman todas las de dentro en orden alfabético; si apunta a un
// archivo, es una sola.
func expresionesDe(p *perfil.Perfil) []string {
	ruta := p.RutaRelativa(p.Personaje.Imagen)
	if ruta == "" {
		return nil
	}
	info, err := os.Stat(ruta)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		return []string{ruta}
	}

	entradas, err := os.ReadDir(ruta)
	if err != nil {
		return nil
	}
	var imgs []string
	for _, e := range entradas {
		if e.IsDir() || !esImagen(e.Name()) {
			continue
		}
		imgs = append(imgs, filepath.Join(ruta, e.Name()))
	}
	// Orden alfabético para que numerar los archivos controle la secuencia.
	sort.Strings(imgs)
	return imgs
}

func esImagen(nombre string) bool {
	switch strings.ToLower(filepath.Ext(nombre)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}

// turnos empareja cada escena con la expresión que le toca y el tramo de tiempo
// que ocupa. Las expresiones se reparten en ciclo: con cuatro imágenes y siete
// escenas, la quinta escena vuelve a la primera imagen.
type turno struct {
	imagen int // índice dentro de expresiones
	inicio float64
	fin    float64
}

func turnosDeExpresion(escenas []proveedor.EscenaRender, tramos []tramo, nImagenes int) []turno {
	if nImagenes <= 0 || len(tramos) == 0 {
		return nil
	}
	if nImagenes == 1 {
		// Una sola imagen: un turno que cubre todo, sin condiciones de tiempo.
		return []turno{{imagen: 0, inicio: 0, fin: 0}}
	}

	var turnos []turno
	idx := 0
	for i, e := range escenas {
		if len(e.Imagenes) == 0 || idx >= len(tramos) {
			continue
		}
		ultimo := idx + len(e.Imagenes) - 1
		if ultimo >= len(tramos) {
			ultimo = len(tramos) - 1
		}
		turnos = append(turnos, turno{
			imagen: i % nImagenes,
			inicio: tramos[idx].inicio.Seconds(),
			fin:    tramos[ultimo].fin.Seconds(),
		})
		idx += len(e.Imagenes)
	}
	// El último turno se estira hasta el final para que no quede un hueco sin
	// personaje por redondeos de la última escena.
	if len(turnos) > 0 {
		turnos[len(turnos)-1].fin = tramos[len(tramos)-1].fin.Seconds() + 1
	}
	return turnos
}
