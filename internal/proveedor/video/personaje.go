package video

import (
	"fmt"
	"os"
	"strings"
	"time"

	"agente-video/internal/perfil"
)

// Superposición de un personaje que acompaña la narración.
//
// No hay sincronía labial: eso exige un modelo y una GPU. Pero sí tenemos los
// tiempos de cada palabra, y con eso basta para que el personaje se mueva
// cuando se habla y se quede quieto cuando no. El ojo lee ese acoplamiento
// como "está narrando", aunque la boca no cambie: es el mismo truco de los
// muñecos de guiñol, donde la cabeza se mueve al hablar y nadie mira la boca.
//
// Sin esa sincronía —un personaje que se mueve todo el rato igual— el efecto se
// pierde y queda una calcomanía animada encima del video.

const (
	// huecoFusion junta palabras seguidas en un mismo tramo de habla. Por
	// debajo de esto la pausa es articulación, no silencio: separarlas haría
	// vibrar al personaje entre sílaba y sílaba.
	huecoFusion = 320 * time.Millisecond
	// maxTramos acota la expresión que se le pasa a ffmpeg. Con narraciones
	// largas serían cientos de términos y el filtro se vuelve ingobernable.
	maxTramos = 40
)

// filtroPersonaje devuelve el fragmento de grafo que superpone al personaje, o
// cadena vacía si el perfil no define ninguno. entradaIdx es el índice de la
// imagen del personaje entre las entradas de ffmpeg.
func filtroPersonaje(p *perfil.Perfil, entradaIdx int, etiquetaEntrada string,
	palabras []palabra, duracion float64,
	expresiones []string, turnos []turno) (filtro, salida string) {

	pj := p.Personaje
	if len(expresiones) == 0 {
		return "", etiquetaEntrada
	}

	altoPct := pj.AltoPct
	if altoPct <= 0 {
		altoPct = 30
	}
	alto := int(float64(p.Formato.Alto) * altoPct / 100)
	margen := pj.Margen
	if margen == 0 {
		margen = p.Formato.Ancho / 18
	}
	opacidad := pj.Opacidad
	if opacidad <= 0 {
		opacidad = 1
	}

	x := posicionX(pj.Posicion, margen)
	y := expresionY(pj.Animacion, margen, palabras, duracion)

	// En ffmpeg una etiqueta de salida solo se puede consumir UNA vez. Como las
	// expresiones rotan, la misma imagen vuelve a usarse en escenas posteriores,
	// y hay que duplicarla con split tantas veces como turnos la reclamen. Sin
	// esto el personaje aparece en las primeras escenas y desaparece después.
	usos := make([]int, len(expresiones))
	for _, t := range turnos {
		usos[t.imagen]++
	}

	var partes []string
	for i := range expresiones {
		if usos[i] == 0 {
			continue // una expresión que no le toca a ninguna escena
		}
		cadena := fmt.Sprintf("[%d:v]%s", entradaIdx+i, cadenaForma(pj, alto, opacidad))
		if usos[i] == 1 {
			partes = append(partes, cadena+fmt.Sprintf("[pj%d_0]", i))
			continue
		}
		var etiquetas strings.Builder
		for k := 0; k < usos[i]; k++ {
			fmt.Fprintf(&etiquetas, "[pj%d_%d]", i, k)
		}
		partes = append(partes, fmt.Sprintf("%s,split=%d%s", cadena, usos[i], etiquetas.String()))
	}

	// Se encadenan tantas superposiciones como turnos, cada una activa solo
	// durante su tramo. Con una única expresión no hay condición de tiempo.
	copia := make([]int, len(expresiones))
	entrada := etiquetaEntrada
	for i, t := range turnos {
		salida := fmt.Sprintf("cpj%d", i)
		enable := ""
		if len(turnos) > 1 {
			enable = fmt.Sprintf(":enable='between(t,%.2f,%.2f)'", t.inicio, t.fin)
		}
		partes = append(partes, fmt.Sprintf("[%s][pj%d_%d]overlay=x=%s:y=%s%s[%s]",
			entrada, t.imagen, copia[t.imagen], x, y, enable, salida))
		copia[t.imagen]++
		entrada = salida
	}

	return strings.Join(partes, ";"), entrada
}

func posicionX(posicion string, margen int) string {
	switch posicion {
	case "abajo-izquierda", "izquierda":
		return fmt.Sprint(margen)
	case "abajo-centro", "centro":
		return "(W-w)/2"
	default: // abajo-derecha
		return fmt.Sprintf("W-w-%d", margen)
	}
}

// expresionY construye el movimiento vertical.
//
// El personaje se apoya en el borde inferior y oscila. En modo "hablar" la
// amplitud se multiplica dentro de los tramos donde hay voz, usando una suma de
// between() que vale 1 mientras se habla y 0 en los silencios.
func expresionY(animacion string, margen int, palabras []palabra, duracion float64) string {
	base := fmt.Sprintf("H-h-%d", margen)

	switch animacion {
	case "ninguna", "":
		return base

	case "respirar":
		// Oscilación lenta y pequeña: parece que está ahí, sin llamar la atención.
		return fmt.Sprintf("'%s+4*sin(2*PI*0.45*t)'", base)

	case "hablar":
		hablando := expresionHabla(palabras, duracion)
		// abs(sin) da un rebote hacia arriba y vuelta, no un vaivén: se lee
		// como énfasis al hablar en lugar de como flotar.
		return fmt.Sprintf("'%s+3*sin(2*PI*0.45*t)-(%s)*9*abs(sin(2*PI*2.3*t))'",
			base, hablando)

	default:
		return base
	}
}

// expresionHabla vale 1 durante los tramos con voz y 0 en los silencios.
func expresionHabla(palabras []palabra, duracion float64) string {
	tramos := tramosDeHabla(palabras)
	if len(tramos) == 0 {
		// Sin tiempos no se puede sincronizar; se asume que se habla siempre,
		// que es mejor que un personaje congelado durante todo el video.
		return "1"
	}
	var terminos []string
	for _, t := range tramos {
		terminos = append(terminos, fmt.Sprintf("between(t,%.2f,%.2f)",
			t.inicio.Seconds(), t.fin.Seconds()))
	}
	// min(...,1) evita que dos tramos solapados sumen 2 y dupliquen la amplitud.
	return "min(" + strings.Join(terminos, "+") + ",1)"
}

// tramosDeHabla agrupa las palabras en bloques continuos de voz.
func tramosDeHabla(palabras []palabra) []tramo {
	if len(palabras) == 0 {
		return nil
	}
	var tramos []tramo
	actual := tramo{inicio: palabras[0].inicio, fin: palabras[0].fin}

	for _, p := range palabras[1:] {
		if p.inicio-actual.fin <= huecoFusion {
			actual.fin = p.fin
			continue
		}
		tramos = append(tramos, actual)
		actual = tramo{inicio: p.inicio, fin: p.fin}
	}
	tramos = append(tramos, actual)

	// Si salieron demasiados, se fusionan los más próximos hasta caber. Una
	// expresión con cientos de términos es peor que perder algo de precisión.
	for len(tramos) > maxTramos {
		tramos = fusionarMasCercanos(tramos)
	}
	return tramos
}

func fusionarMasCercanos(tramos []tramo) []tramo {
	if len(tramos) < 2 {
		return tramos
	}
	mejor, menor := 0, tramos[1].inicio-tramos[0].fin
	for i := 1; i < len(tramos)-1; i++ {
		if d := tramos[i+1].inicio - tramos[i].fin; d < menor {
			mejor, menor = i, d
		}
	}
	fusionado := tramo{inicio: tramos[mejor].inicio, fin: tramos[mejor+1].fin}
	out := append([]tramo{}, tramos[:mejor]...)
	out = append(out, fusionado)
	return append(out, tramos[mejor+2:]...)
}

// cadenaForma prepara la imagen del personaje según la forma pedida.
//
// La forma importa más de lo que parece. Un PNG bien recortado es lo ideal,
// pero exige que alguien lo prepare: los generadores libres ignoran a menudo la
// instrucción de fondo croma, y cuando la respetan suelen vestir al personaje
// del mismo color, con lo que el recorte se come la ropa. Comprobado.
//
// Por eso el modo por defecto es el círculo: funciona con CUALQUIER imagen
// —una foto tuya, un avatar, un fotograma— sin preparación previa, y en video
// vertical se lee como una decisión de diseño y no como un parche.
func cadenaForma(pj perfil.Personaje, alto int, opacidad float64) string {
	alfa := fmt.Sprintf("colorchannelmixer=aa=%.2f", opacidad)

	switch pj.Forma {
	case "recorte":
		// La imagen ya trae su propia transparencia.
		return fmt.Sprintf("scale=-1:%d,format=rgba,%s", alto, alfa)

	case "croma":
		color := pj.ColorCroma
		if color == "" {
			color = "0x00B140"
		}
		// despill quita el reflejo verde que queda en los bordes del pelo.
		return fmt.Sprintf(
			"scale=-1:%d,colorkey=%s:0.30:0.12,despill=type=green:mix=0.5,format=rgba,%s",
			alto, color, alfa)

	case "tarjeta":
		return fmt.Sprintf("scale=-1:%d,format=rgba,%s", alto, alfa)

	default: // "circulo"
		// Cuadrar por el lado corto y centrar arriba: en un retrato la cara
		// está en el tercio superior, y centrar por el medio la decapita.
		return fmt.Sprintf(
			"crop='min(iw,ih)':'min(iw,ih)':'(iw-min(iw,ih))/2':'(ih-min(iw,ih))/6',"+
				"scale=%d:%d,format=rgba,"+
				// Máscara circular sobre el canal alfa, con un borde suave de
				// dos píxeles para que no quede aserrado.
				"geq=r='r(X,Y)':g='g(X,Y)':b='b(X,Y)':"+
				"a='255*clip((%d/2-sqrt(pow(X-%d/2,2)+pow(Y-%d/2,2)))/2,0,1)',%s",
			alto, alto, alto, alto, alto, alfa)
	}
}

// MargenSubtitulos devuelve el margen inferior que deben respetar los
// subtítulos para no quedar debajo del personaje.
//
// Sin esto el personaje se dibuja encima del texto y lo corta. Es un fallo que
// no da ningún error y solo se ve mirando el video terminado, así que conviene
// calcularlo en vez de dejarlo a que alguien acierte con los números.
func MargenSubtitulos(p *perfil.Perfil) int {
	pj := p.Personaje
	if pj.Imagen == "" || pj.Animacion == "" {
		return p.Subtitulos.MargenV
	}
	if _, err := os.Stat(p.RutaRelativa(pj.Imagen)); err != nil {
		return p.Subtitulos.MargenV
	}
	// Un personaje centrado abajo tapa el texto sí o sí; a los lados solo
	// estorba si el texto llega hasta abajo, que es lo habitual en vertical.
	altoPct := pj.AltoPct
	if altoPct <= 0 {
		altoPct = 30
	}
	alto := int(float64(p.Formato.Alto) * altoPct / 100)
	margen := pj.Margen
	if margen == 0 {
		margen = p.Formato.Ancho / 18
	}
	// El rebote de la animación sube el personaje unos píxeles; se deja aire.
	necesario := margen + alto + 40
	if necesario > p.Subtitulos.MargenV {
		return necesario
	}
	return p.Subtitulos.MargenV
}
