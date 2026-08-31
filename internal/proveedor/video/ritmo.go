package video

import (
	"math"
	"time"

	"agente-video/internal/proveedor"
)

// Este archivo decide cuánto dura cada imagen en pantalla.
//
// La versión simple —repartir la duración total en partes iguales— produce un
// video donde las imágenes cambian a destiempo del relato: una idea corta se
// queda fija demasiado y una larga se corta a media frase. Se nota, y se nota
// como error de edición.
//
// Como whisper ya nos da el tiempo de cada palabra, podemos hacer lo correcto:
// alinear cada escena con el tramo de audio donde realmente se narra, y repartir
// ese tramo entre sus planos. Así la imagen cambia cuando cambia la idea.

type tramo struct {
	inicio time.Duration
	fin    time.Duration
}

func (t tramo) dura() time.Duration { return t.fin - t.inicio }

// repartir devuelve un tramo por cada imagen del video, en orden.
//
// palabras son los tiempos reales de la narración. Si vienen vacías (subtítulos
// desactivados) se cae al reparto uniforme, que es lo único posible sin datos.
func repartir(escenas []proveedor.EscenaRender, palabras []palabra,
	total time.Duration, minSeg, maxSeg float64) []tramo {

	imagenes := 0
	for _, e := range escenas {
		imagenes += len(e.Imagenes)
	}
	if imagenes == 0 {
		return nil
	}
	if len(palabras) == 0 {
		return uniforme(imagenes, total)
	}

	limites := limitesDeEscena(escenas, palabras, total)

	var tramos []tramo
	for i, e := range escenas {
		n := len(e.Imagenes)
		if n == 0 {
			continue
		}
		ini, fin := limites[i], limites[i+1]
		paso := float64(fin-ini) / float64(n)
		for j := 0; j < n; j++ {
			tramos = append(tramos, tramo{
				inicio: ini + time.Duration(float64(j)*paso),
				fin:    ini + time.Duration(float64(j+1)*paso),
			})
		}
	}

	return acotar(tramos, total, minSeg, maxSeg)
}

// limitesDeEscena traduce las escenas del guion a instantes del audio.
//
// No se comparan textos: whisper transcribe lo que oye y no siempre coincide
// con lo que se le dio a sintetizar ("cuarenta" puede volver como "40"). Lo que
// sí se mantiene es la proporción, así que se reparte la línea de tiempo según
// el peso en palabras de cada escena. Devuelve len(escenas)+1 marcas.
func limitesDeEscena(escenas []proveedor.EscenaRender, palabras []palabra,
	total time.Duration) []time.Duration {

	pesos := make([]int, len(escenas))
	suma := 0
	for i, e := range escenas {
		pesos[i] = contarPalabras(e.Narracion)
		if pesos[i] == 0 {
			pesos[i] = 1 // una escena sin narración igual ocupa su turno
		}
		suma += pesos[i]
	}

	limites := make([]time.Duration, len(escenas)+1)
	limites[0] = 0
	acumulado := 0
	for i, peso := range pesos {
		acumulado += peso
		if i == len(pesos)-1 {
			limites[i+1] = total // la última escena llega hasta el final del audio
			break
		}
		// Índice de la palabra transcrita donde debería caer el corte.
		idx := int(math.Round(float64(acumulado) / float64(suma) * float64(len(palabras))))
		switch {
		case idx <= 0:
			limites[i+1] = palabras[0].inicio
		case idx >= len(palabras):
			limites[i+1] = total
		default:
			// El corte va en el silencio entre dos palabras, no encima de una.
			limites[i+1] = palabras[idx-1].fin
		}
		if limites[i+1] < limites[i] {
			limites[i+1] = limites[i]
		}
	}
	return limites
}

// acotar corrige los tramos demasiado cortos o demasiado largos y reajusta para
// que la suma siga cuadrando exactamente con la duración del audio. Sin esto,
// una escena con mucha narración y un solo plano deja la imagen congelada.
func acotar(tramos []tramo, total time.Duration, minSeg, maxSeg float64) []tramo {
	if len(tramos) == 0 {
		return tramos
	}
	min := time.Duration(minSeg * float64(time.Second))
	max := time.Duration(maxSeg * float64(time.Second))

	// Con tantas imágenes, el mínimo puede ser imposible de respetar; en ese
	// caso manda la duración del audio y se reparte lo que haya.
	if min > 0 && time.Duration(len(tramos))*min > total {
		return uniforme(len(tramos), total)
	}

	duraciones := make([]time.Duration, len(tramos))
	var suma time.Duration
	for i, t := range tramos {
		d := t.dura()
		if min > 0 && d < min {
			d = min
		}
		if max > 0 && d > max {
			d = max
		}
		duraciones[i] = d
		suma += d
	}

	// Reescalar para volver a cuadrar con el audio.
	if suma > 0 && suma != total {
		factor := float64(total) / float64(suma)
		for i := range duraciones {
			duraciones[i] = time.Duration(float64(duraciones[i]) * factor)
		}
	}

	salida := make([]tramo, len(tramos))
	var cursor time.Duration
	for i, d := range duraciones {
		salida[i] = tramo{inicio: cursor, fin: cursor + d}
		cursor += d
	}
	salida[len(salida)-1].fin = total // absorber el redondeo al final
	return salida
}

func uniforme(n int, total time.Duration) []tramo {
	tramos := make([]tramo, n)
	paso := total / time.Duration(n)
	for i := range tramos {
		tramos[i] = tramo{inicio: time.Duration(i) * paso, fin: time.Duration(i+1) * paso}
	}
	tramos[n-1].fin = total
	return tramos
}

func contarPalabras(s string) int {
	n, dentro := 0, false
	for _, r := range s {
		esEspacio := r == ' ' || r == '\n' || r == '\t' || r == '\r'
		if esEspacio {
			dentro = false
			continue
		}
		if !dentro {
			n++
			dentro = true
		}
	}
	return n
}

// zoomDeEncuadre ajusta la intensidad del movimiento al tipo de plano. Un
// primer plano con el mismo zoom que un plano general se siente brusco: hay
// menos superficie donde el movimiento se reparta.
func zoomDeEncuadre(base float64, encuadre string) float64 {
	switch encuadre {
	case "detalle", "cercano":
		return 1 + (base-1)*0.55
	case "cenital":
		return 1 + (base-1)*0.8
	default: // general, medio, o sin especificar
		return base
	}
}
