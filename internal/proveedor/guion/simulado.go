package guion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

// Simulado devuelve un guion fijo sin llamar a ninguna API.
//
// El guionista es la única etapa que cuesta dinero: imágenes, voz, subtítulos y
// montaje son gratis. Al ajustar el personaje, los subtítulos, la música o el
// ritmo hacen falta muchas pasadas, y pagar un guion nuevo cada vez para tirarlo
// a los diez segundos no tiene sentido.
//
// El texto está pensado para que las pruebas sean representativas: escenas de
// duración desigual, un sujeto recurrente, encuadres variados y una pausa larga
// —eso último ejercita el corte de línea de los subtítulos y los tramos de
// silencio del personaje, que es donde suelen aparecer los fallos.
type Simulado struct {
	// Archivo, si se indica, carga el guion de ahí en vez del incorporado.
	// Sirve para reproducir un caso concreto que falló.
	Archivo string
}

func NuevoSimulado(archivo string) *Simulado { return &Simulado{Archivo: archivo} }

func (s *Simulado) Nombre() string {
	if s.Archivo != "" {
		return "simulado:" + s.Archivo
	}
	return "simulado (sin costo)"
}

func (s *Simulado) Generar(ctx context.Context, p *perfil.Perfil, tema string) (*proveedor.GuionGenerado, error) {
	if s.Archivo != "" {
		datos, err := os.ReadFile(s.Archivo)
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer el guion %s: %w", s.Archivo, err)
		}
		var g proveedor.GuionGenerado
		if err := json.Unmarshal(datos, &g); err != nil {
			return nil, fmt.Errorf("guion inválido en %s: %w", s.Archivo, err)
		}
		if len(g.Escenas) == 0 {
			return nil, fmt.Errorf("el guion de %s no trae escenas", s.Archivo)
		}
		g.Normalizar()
		return &g, nil
	}

	g := guionDePrueba()
	// Se recorta o se repite para respetar el número de escenas del perfil, de
	// modo que la prueba refleje el formato real que se va a producir.
	g.Escenas = ajustarEscenas(g.Escenas, p.Guion.Escenas)
	g.Normalizar()
	return g, nil
}

func ajustarEscenas(base []proveedor.Escena, quiero int) []proveedor.Escena {
	if quiero <= 0 || len(base) == 0 {
		return base
	}
	out := make([]proveedor.Escena, 0, quiero)
	for i := 0; i < quiero; i++ {
		out = append(out, base[i%len(base)])
	}
	return out
}

func guionDePrueba() *proveedor.GuionGenerado {
	return &proveedor.GuionGenerado{
		Titulo:      "Guion de prueba",
		Descripcion: "Texto fijo para probar el montaje sin gastar créditos.",
		Hashtags:    []string{"prueba", "montaje"},
		Escenas: []proveedor.Escena{
			{
				Narracion: "Hay una puerta en el sótano de esta casa que nadie abre desde hace treinta años.",
				Planos: []proveedor.Plano{
					{Prompt: "an old wooden cellar door with peeling paint, dim light from above, dust in the air", Encuadre: "medio", Sujeto: "puerta"},
					{Prompt: "extreme close-up of a rusted iron door handle, cobwebs, cold blue light", Encuadre: "detalle"},
				},
			},
			{
				Narracion: "La llave se perdió el mismo invierno en que murió el abuelo.",
				Planos: []proveedor.Plano{
					{Prompt: "a snowy window at dusk in an old house, warm lamp reflected in the glass", Encuadre: "general"},
				},
			},
			{
				Narracion: "Nadie llamó a un cerrajero. Nadie preguntó por qué. Simplemente dejaron de bajar.",
				Planos: []proveedor.Plano{
					{Prompt: "an empty wooden staircase descending into darkness, single bare bulb", Encuadre: "general"},
					{Prompt: "close-up of a family photograph on a sideboard, faces slightly out of focus", Encuadre: "cercano"},
					{Prompt: "overhead view of dusty floorboards with a faint rectangular mark where furniture stood", Encuadre: "cenital"},
				},
			},
			{
				Narracion: "Pero cada invierno, cuando la casa se enfría, la puerta suena.",
				Planos: []proveedor.Plano{
					{Prompt: "an old wooden cellar door with peeling paint, dim light from above, dust in the air", Encuadre: "medio", Sujeto: "puerta"},
				},
			},
			{
				Narracion: "Y este año alguien encontró la llave en un cajón que llevaba décadas vacío. ¿Tú la abrirías?",
				Planos: []proveedor.Plano{
					{Prompt: "a single old brass key resting on worn fabric inside a drawer, shallow depth of field", Encuadre: "detalle"},
					{Prompt: "a person's hand hovering over a door handle, hesitating, dark hallway", Encuadre: "cercano"},
				},
			},
		},
	}
}
