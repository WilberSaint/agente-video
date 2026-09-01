package guion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

// Carpeta busca un guion ya escrito en disco antes de pagar por uno nuevo.
//
// Existe por una observación incómoda: el modelo que escribe los guiones por
// API es el mismo con el que se conversa al desarrollar. Estando delante, el
// guion se puede escribir en la conversación y guardar aquí, sin coste de API.
// Lo que la API compra no es la calidad del texto: es que a las tres de la
// mañana haya alguien capaz de escribirlo.
//
// Así que se pueden dejar guiones escritos de antemano —uno por tema del
// banco— y el horario los consume gratis. Cuando se acaben, entra el respaldo
// y la producción no se detiene.
type Carpeta struct {
	dir      string
	respaldo proveedor.Guionista
	// Aviso cuenta de dónde salió el guion. Sin esto no se distingue una noche
	// gratis de una que costó dinero.
	Aviso func(string, ...any)
}

func NuevoCarpeta(dir string, respaldo proveedor.Guionista) *Carpeta {
	return &Carpeta{dir: dir, respaldo: respaldo}
}

func (c *Carpeta) Nombre() string {
	if c.respaldo == nil {
		return "carpeta:" + c.dir
	}
	return "carpeta:" + c.dir + " (respaldo: " + c.respaldo.Nombre() + ")"
}

func (c *Carpeta) avisar(f string, a ...any) {
	if c.Aviso != nil {
		c.Aviso(f, a...)
	}
}

func (c *Carpeta) Generar(ctx context.Context, p *perfil.Perfil, tema string) (*proveedor.GuionGenerado, error) {
	ruta := filepath.Join(c.dir, Ranura(tema)+".json")

	datos, err := os.ReadFile(ruta)
	if err == nil {
		var g proveedor.GuionGenerado
		if err := json.Unmarshal(datos, &g); err != nil {
			return nil, proveedor.Permanente(fmt.Errorf("%s no es un guion válido: %w", ruta, err))
		}
		if len(g.Escenas) == 0 {
			return nil, proveedor.Permanente(fmt.Errorf("%s no tiene escenas", ruta))
		}
		g.Normalizar()
		c.avisar("guion tomado de %s (sin coste de API)", ruta)
		return &g, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	if c.respaldo == nil {
		return nil, proveedor.Permanente(fmt.Errorf(
			"no hay guion para %q en %s y no se configuró respaldo.\n\n"+
				"Déjalo en %s, o pon guion.respaldo en el perfil", tema, c.dir, ruta))
	}
	c.avisar("no había guion para %q en %s; se escribe con %s", tema, c.dir, c.respaldo.Nombre())
	return c.respaldo.Generar(ctx, p, tema)
}

// Ranura convierte un tema en el nombre de archivo que le corresponde. Es la
// única regla que une el banco de temas con los guiones escritos a mano: si
// cambia, los guiones dejan de encontrarse.
func Ranura(tema string) string {
	var b strings.Builder
	espacio := false
	for _, r := range strings.ToLower(strings.TrimSpace(tema)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if espacio && b.Len() > 0 {
				b.WriteRune('-')
			}
			espacio = false
			if plana, ok := sinAcento[r]; ok {
				r = plana
			}
			b.WriteRune(r)
		default:
			espacio = true
		}
	}
	s := b.String()
	if len(s) > 60 {
		s = s[:60]
	}
	return strings.Trim(s, "-")
}

var sinAcento = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
}

var _ proveedor.Guionista = (*Carpeta)(nil)
