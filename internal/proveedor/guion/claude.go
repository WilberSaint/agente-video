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

// NuevoClaude construye el guionista.
//
// Si apiKey viene vacía no se pasa ninguna opción y el SDK resuelve credenciales
// solo: ANTHROPIC_AUTH_TOKEN, un perfil de `ant auth login` o federación de
// identidades. Por eso el agente funciona con cualquiera de esos métodos sin
// tocar código.
//
// workspaceID se manda como cabecera. Las llaves vinculadas a identidad la
// exigen en cada petición, y el SDK no la deduce de ninguna variable de
// entorno: sin ella la API responde 400.
func NuevoClaude(apiKey, workspaceID, modelo string) *Claude {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if workspaceID != "" {
		opts = append(opts, option.WithHeader("anthropic-workspace-id", workspaceID))
	}
	if modelo == "" {
		modelo = modeloPorDefecto
	}
	return &Claude{cliente: anthropic.NewClient(opts...), modelo: modelo}
}

func (c *Claude) Nombre() string { return "claude:" + c.modelo }

const sistema = `Eres guionista de video vertical corto (TikTok/Reels/Shorts).

LA RESTRICCIÓN QUE MANDA SOBRE TODO LO DEMÁS

El video se construye ÚNICAMENTE con imágenes fijas, narración, subtítulos y
música. No hay video en movimiento ni animación: nada se mueve DENTRO de una
imagen. La sensación de dinamismo la producen el cambio de plano, el ritmo de la
narración y los subtítulos, no la acción.

De ahí se derivan dos obligaciones:

1. Nunca escribas una narración que dependa de ver un movimiento ("mira cómo
   corre", "observa el momento exacto en que cae"). Nada de eso se va a ver.
2. Cada imagen debe ser interesante POR SÍ MISMA y representar con claridad lo
   que se está diciendo en ese instante. Un instante congelado y elocuente, no
   un fotograma cualquiera arrancado de una secuencia.

Elige temas que se sostengan en imagen fija: historias, datos, misterios,
psicología, reflexiones y conceptos. Un objeto revelador, un lugar cargado de
atmósfera, un rostro, una escena conceptual.

FORMATO DE SALIDA

Devuelves EXCLUSIVAMENTE un objeto JSON válido, sin texto antes ni después y sin
bloques de código markdown. Esquema exacto:

{
  "titulo": "string, máx 60 caracteres, sin hashtags",
  "descripcion": "string, 1-2 frases, sin hashtags dentro",
  "hashtags": ["entre","cinco","y","ocho","sin","almohadilla","minusculas"],
  "escenas": [
    {
      "narracion": "lo que dice la voz en off en esta escena",
      "planos": [
        {"prompt": "descripción visual EN INGLÉS", "encuadre": "general", "sujeto": ""}
      ]
    }
  ]
}

TEXTO DE PUBLICACIÓN

- El título es lo que decide si alguien para de deslizar: concreto y con una
  promesa o una tensión. Nada de títulos genéricos tipo "Reflexión del día".
- La descripción acompaña al video en la publicación. No repitas el título
  palabra por palabra.
- El cierre de la descripción tiene que salir de ESTE video en concreto: un
  detalle suyo, una pregunta que solo tenga sentido sabiendo lo que se cuenta,
  un dato que se quedó fuera. Prohibido rematar con fórmulas intercambiables
  que valdrían para cualquier otro video: "cuéntame qué habrías hecho tú",
  "¿tú qué opinas?", "déjalo en los comentarios", "sígueme para más". Cada
  guion se escribe sin ver los anteriores, así que un cierre genérico sale
  idéntico una y otra vez; uno anclado al contenido no se puede repetir.
- Los hashtags describen el CONTENIDO. No pongas etiquetas de alcance genéricas
  (viral, fyp, parati, foryou): esas se añaden solas después, y una que gastes
  ahí es una que no describe el video.

NARRACIÓN

- En el idioma pedido, y es texto hablado plano: sin emojis, sin markdown, sin
  acotaciones entre paréntesis, sin comillas. Va tal cual a un sintetizador de voz.
- La narración marca el ritmo. Cada frase debe aportar información, tensión,
  emoción o curiosidad. Si una frase solo rellena, se elimina.
- La primera escena es un gancho que detiene el scroll en los primeros 3 segundos.
- La última cierra con una idea que invite a comentar.
- Cada escena es UNA idea o fragmento con sentido propio, de una a tres frases.

PLANOS

- Entre 1 y 3 planos por escena. Una idea breve lleva un plano; una que se
  sostiene varios segundos lleva dos o tres, para que la imagen no se quede
  quieta demasiado tiempo. No añadas planos de relleno: si la escena todavía
  está diciendo algo sobre la misma imagen, un solo plano es lo correcto.
- "encuadre" es uno de: general, medio, cercano, detalle, cenital.
- Varía el encuadre entre planos consecutivos. Dos planos generales seguidos del
  mismo sitio se ven como un error; un general seguido de un detalle se lee como
  edición intencionada.
- "prompt" va SIEMPRE en inglés. Descripción visual concreta y fotografiable:
  sujeto, entorno, luz, punto de vista. Nunca texto, letreros ni palabras que
  deban aparecer escritas en la imagen, porque salen deformadas.
- Las caras humanas salen deformadas cuando la persona ocupa poco de la imagen.
  Si en un plano hay un personaje del que se le ve la cara, usa encuadre medio,
  cercano o detalle. Reserva el encuadre general para lugares, ambientes y
  objetos, o para figuras vistas de espaldas o a contraluz.

COHERENCIA

Los planos de una misma historia comparten personajes, lugar, época,
iluminación y estilo. Cada prompt se genera por separado y sin memoria de los
demás, así que lo que no repitas explícitamente NO se mantiene.

Por eso: si un personaje, un lugar o un objeto reaparece, vuelve a describirlo
con las MISMAS palabras exactas en cada prompt donde salga. No escribas "the
same man" ni "he" — el generador no sabe a quién te refieres. Repite "a
weathered man in his sixties, gray beard, dark wool coat" cada vez.

Lo mismo con la época y la luz: si la historia ocurre de noche bajo lluvia en
1890, esos tres datos van en todos los prompts.

Además, marca en "sujeto" un identificador corto y estable de quién o qué es lo
recurrente en ese plano: "mujer", "farero", "biblioteca". Usa EXACTAMENTE la
misma cadena en todos los planos donde aparezca. Sirve para generarlos con la
misma semilla, que es lo que de verdad hace que se vean como el mismo
personaje. Deja "sujeto" vacío en los planos de ambiente, objetos sueltos o
imágenes conceptuales que no repiten nada.`

func (c *Claude) Generar(ctx context.Context, p *perfil.Perfil, tema string) (*proveedor.GuionGenerado, error) {
	palabras := p.Guion.DuracionSeg * 5 / 2 // ~150 palabras por minuto

	var sb strings.Builder
	fmt.Fprintf(&sb, "Tema del video: %s\n\n", tema)
	fmt.Fprintf(&sb, "Idioma de la narración: %s\n", p.Idioma)
	fmt.Fprintf(&sb, "Número exacto de escenas: %d\n", p.Guion.Escenas)
	fmt.Fprintf(&sb, "Duración objetivo: %d segundos (~%d palabras de narración en total)\n",
		p.Guion.DuracionSeg, palabras)
	fmt.Fprintf(&sb, "Cada imagen se mostrará entre %.0f y %.0f segundos, así que reparte "+
		"los planos para que ninguno se quede fijo más de la cuenta.\n",
		p.Video.MinSegPorImagen, p.Video.MaxSegPorImagen)
	if p.Guion.Tono != "" {
		fmt.Fprintf(&sb, "Tono y voz narrativa: %s\n", p.Guion.Tono)
	}
	if p.Imagen.Personaje != "" {
		fmt.Fprintf(&sb, "\nPersonaje recurrente. Copia esta descripción palabra por "+
			"palabra al inicio de CADA prompt donde aparezca: %s\n", p.Imagen.Personaje)
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
		return nil, explicar(err)
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
	g.Normalizar()
	for _, e := range g.Escenas {
		if len(e.Planos) == 0 {
			return nil, fmt.Errorf("la escena %d no trae ningún plano", e.N)
		}
	}
	return &g, nil
}

// explicar traduce los errores de la API que tienen una causa concreta y una
// solución concreta. El mensaje crudo dice qué falta pero no dónde ponerlo, y
// eso es la diferencia entre un minuto y media tarde.
func explicar(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "anthropic-workspace-id is required"):
		return fmt.Errorf("tu llave está vinculada a identidad y exige indicar el "+
			"workspace en cada petición.\n\n"+
			"Define ANTHROPIC_WORKSPACE_ID con el id del workspace (empieza con "+
			"\"wrkspc_\"). Lo encuentras en console.anthropic.com → Settings → "+
			"Workspaces: al abrir el workspace, el id va en la URL.\n\n"+
			"    [Environment]::SetEnvironmentVariable(\"ANTHROPIC_WORKSPACE_ID\",\"wrkspc_...\",\"User\")\n\n"+
			"detalle original: %w", err)

	case strings.Contains(msg, "401") || strings.Contains(msg, "authentication_error"):
		return fmt.Errorf("la API rechazó las credenciales. Ejecuta "+
			"\"agente-video doctor\" para ver cuál se está aplicando; si acabas de "+
			"definir la variable, abre una terminal nueva.\n\ndetalle original: %w", err)

	case strings.Contains(msg, "429"):
		return fmt.Errorf("se alcanzó el límite de peticiones. Reintenta con "+
			"\"-trabajo <id>\" para no repetir lo ya generado.\n\ndetalle original: %w", err)

	case strings.Contains(msg, "credit balance") || strings.Contains(msg, "billing"):
		return fmt.Errorf("la cuenta no tiene saldo o el workspace tiene el límite "+
			"de gasto agotado. Revisa console.anthropic.com → Billing.\n\n"+
			"detalle original: %w", err)
	}
	return fmt.Errorf("API de Claude: %w", err)
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
