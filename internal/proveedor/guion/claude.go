// Package guion implementa el guionista sobre la API de Claude.
package guion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"agente-video/internal/perfil"
	"agente-video/internal/proveedor"
)

const modeloPorDefecto = "claude-opus-5"

type Claude struct {
	cliente anthropic.Client
	modelo  string
}

// NuevoClaude construye el guionista. Si apiKey viene vacía el SDK resuelve
// credenciales solo (ANTHROPIC_API_KEY, ANTHROPIC_AUTH_TOKEN o perfil de `ant auth login`).
func NuevoClaude(apiKey, modelo string) *Claude {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if modelo == "" {
		modelo = modeloPorDefecto
	}
	return &Claude{cliente: anthropic.NewClient(opts...), modelo: modelo}
}

func (c *Claude) Nombre() string { return "claude:" + c.modelo }

const sistema = `Eres guionista de video vertical corto (TikTok/Reels/Shorts).

Devuelves EXCLUSIVAMENTE un objeto JSON válido, sin texto antes ni después y sin
bloques de código markdown. Esquema exacto:

{
  "titulo": "string, máx 60 caracteres",
  "descripcion": "string, 1-2 frases para la publicación",
  "hashtags": ["sin","almohadilla","minusculas"],
  "escenas": [
    {"n": 1, "narracion": "lo que dice la voz en off", "prompt": "descripción visual EN INGLÉS"}
  ]
}

Reglas irrompibles:
- "narracion" va en el idioma pedido y es texto hablado plano: sin emojis, sin
  markdown, sin acotaciones entre paréntesis, sin comillas. Se envía tal cual a
  un sintetizador de voz.
- "prompt" va SIEMPRE en inglés, es una descripción visual concreta y fotografiable
  (sujeto, entorno, luz, encuadre). Nunca incluye texto, letreros ni palabras que
  deban aparecer escritas en la imagen.
- Cada escena aporta una imagen visualmente distinta de las demás.
- La primera narración es un gancho que detiene el scroll en los primeros 3 segundos.
- La última cierra con una idea que invite a comentar.
- Respeta exactamente el número de escenas pedido.`

func (c *Claude) Generar(ctx context.Context, p *perfil.Perfil, tema string) (*proveedor.GuionGenerado, error) {
	palabras := p.Guion.DuracionSeg * 5 / 2 // ~150 palabras por minuto

	var sb strings.Builder
	fmt.Fprintf(&sb, "Tema del video: %s\n\n", tema)
	fmt.Fprintf(&sb, "Idioma de la narración: %s\n", p.Idioma)
	fmt.Fprintf(&sb, "Número exacto de escenas: %d\n", p.Guion.Escenas)
	fmt.Fprintf(&sb, "Duración objetivo: %d segundos (~%d palabras de narración en total)\n",
		p.Guion.DuracionSeg, palabras)
	if p.Guion.Tono != "" {
		fmt.Fprintf(&sb, "Tono y voz narrativa: %s\n", p.Guion.Tono)
	}
	if p.Imagen.Personaje != "" {
		fmt.Fprintf(&sb, "\nPersonaje recurrente que debe aparecer descrito de forma idéntica "+
			"al inicio de cada prompt visual: %s\n", p.Imagen.Personaje)
	}
	if p.Guion.Extra != "" {
		fmt.Fprintf(&sb, "\nInstrucciones adicionales del canal: %s\n", p.Guion.Extra)
	}

	stream := c.cliente.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.modelo),
		MaxTokens: 16000,
		System: []anthropic.TextBlockParam{{
			Text:         sistema,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(sb.String())),
		},
	})

	mensaje := anthropic.Message{}
	for stream.Next() {
		if err := mensaje.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("acumulando respuesta: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("API de Claude: %w", err)
	}
	if mensaje.StopReason == anthropic.StopReasonRefusal {
		return nil, fmt.Errorf("el modelo rechazó el tema (categoría %q): %s",
			mensaje.StopDetails.Category, mensaje.StopDetails.Explanation)
	}

	var texto strings.Builder
	for _, bloque := range mensaje.Content {
		if tb, ok := bloque.AsAny().(anthropic.TextBlock); ok {
			texto.WriteString(tb.Text)
		}
	}
	crudo := texto.String()
	if strings.TrimSpace(crudo) == "" {
		return nil, fmt.Errorf("la respuesta no contiene texto (stop_reason=%s)", mensaje.StopReason)
	}

	var g proveedor.GuionGenerado
	if err := json.Unmarshal([]byte(extraerJSON(crudo)), &g); err != nil {
		return nil, fmt.Errorf("el modelo no devolvió JSON válido: %w\n---\n%s", err, recortar(crudo, 600))
	}
	if len(g.Escenas) == 0 {
		return nil, fmt.Errorf("el guion no trae escenas")
	}
	for i := range g.Escenas {
		g.Escenas[i].N = i + 1
	}
	return &g, nil
}

// extraerJSON tolera que el modelo envuelva la respuesta en ```json ... ```
// o la acompañe de texto, quedándose con el primer objeto de nivel superior.
func extraerJSON(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		resto := s[i+3:]
		resto = strings.TrimPrefix(resto, "json")
		if f := strings.Index(resto, "```"); f >= 0 {
			s = strings.TrimSpace(resto[:f])
		}
	}
	ini := strings.Index(s, "{")
	fin := strings.LastIndex(s, "}")
	if ini >= 0 && fin > ini {
		return s[ini : fin+1]
	}
	return s
}

func recortar(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
