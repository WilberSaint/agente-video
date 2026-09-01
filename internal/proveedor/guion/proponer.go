package guion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"agente-video/internal/perfil"
)

// Proponer pide ideas nuevas para un canal.
//
// Sin esto, un banco de temas vacío a las tres de la mañana significa una noche
// perdida. Con esto, el agente puede seguir produciendo aunque nadie haya
// escrito nada en una semana.
//
// Se le pasan los temas ya usados para que no repita: un canal que publica la
// misma idea dos veces se nota más que uno que publica poco.
func (c *Claude) Proponer(ctx context.Context, p *perfil.Perfil, cuantos int,
	yaUsados []string) ([]string, error) {

	if cuantos <= 0 {
		cuantos = 5
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Canal: %s\n", p.Nombre)
	if p.Guion.Tono != "" {
		fmt.Fprintf(&sb, "Tono y voz narrativa: %s\n", p.Guion.Tono)
	}
	if p.Guion.Extra != "" {
		fmt.Fprintf(&sb, "Instrucciones del canal: %s\n", p.Guion.Extra)
	}
	fmt.Fprintf(&sb, "Duración de cada video: unos %d segundos.\n", p.Guion.DuracionSeg)
	fmt.Fprintf(&sb, "\nPropón %d temas nuevos.\n", cuantos)

	if len(yaUsados) > 0 {
		sb.WriteString("\nYa se publicaron estos, así que no los repitas ni " +
			"propongas variaciones cercanas:\n")
		for _, t := range yaUsados {
			fmt.Fprintf(&sb, "- %s\n", t)
		}
	}

	stream := c.cliente.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.modelo),
		MaxTokens: 4000,
		System: []anthropic.TextBlockParam{{
			Text:         sistemaProponer,
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
		return nil, explicar(err)
	}

	var texto strings.Builder
	for _, b := range mensaje.Content {
		if tb, ok := b.AsAny().(anthropic.TextBlock); ok {
			texto.WriteString(tb.Text)
		}
	}

	var salida struct {
		Temas []string `json:"temas"`
	}
	if err := json.Unmarshal([]byte(extraerJSON(texto.String())), &salida); err != nil {
		return nil, fmt.Errorf("el modelo no devolvió JSON válido: %w\n---\n%s",
			err, recortar(texto.String(), 400))
	}
	if len(salida.Temas) == 0 {
		return nil, fmt.Errorf("no propuso ningún tema")
	}
	return salida.Temas, nil
}

const sistemaProponer = `Propones temas para un canal de video vertical corto.

Devuelves EXCLUSIVAMENTE un objeto JSON válido, sin texto antes ni después y sin
bloques de código markdown:

{"temas": ["primer tema", "segundo tema"]}

Cada tema es una frase corta que describe DE QUÉ VA el video, no su título.
"por qué olvidamos los sueños al despertar" sirve; "Los sueños" no dice nada y
"¡DESCUBRE EL SECRETO DE LOS SUEÑOS!" es un titular, no un tema.

Reglas:
- Concreto y acotado: uno debe caber en el tiempo que dura el video. "La
  historia de Roma" no cabe; "por qué Roma abandonó Britania de un día para
  otro" sí.
- Que se pueda contar con imágenes fijas. El canal no produce movimiento: los
  temas deben sostenerse en escenarios, objetos, rostros y momentos congelados.
- Variados entre sí. Cinco temas del mismo subgénero agotan el canal.
- Ajustados al tono del canal, no a lo que esté de moda.
- Nada que exija dato exacto y verificable —cifras, fechas precisas,
  declaraciones atribuidas— porque el guion se escribe sin poder comprobarlo y
  publicar un dato falso cuesta más que no publicar.

Escribe en español.`
