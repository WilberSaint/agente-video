package video

import (
	"strings"
	"unicode"

	"agente-video/internal/proveedor"
)

// corregirConGuion arregla lo que whisper oyó mal.
//
// Whisper transcribe el audio sin saber qué se dijo, así que se equivoca:
// escribió "su rija" donde la narración decía "su hija". Pero nosotros SÍ
// sabemos el texto exacto —lo escribió el guionista y se lo dimos al
// sintetizador—, así que la transcripción solo hace falta para los tiempos.
// Las palabras las pone el guion.
//
// Se alinean las dos secuencias y se sustituye cada palabra por la del guion
// conservando su tiempo. Alinear en vez de comparar posición a posición es
// necesario porque whisper añade y se salta palabras, y escribe "40" donde la
// narración dice "cuarenta": sin alineación, un solo desajuste desplazaría
// todo lo que viene detrás.
//
// Devuelve cuántas palabras cambió y si el resultado es de fiar. Si las dos
// secuencias no se parecen, algo va mal —otro audio, otro idioma— y es más
// seguro quedarse con lo que oyó whisper que imponer un texto que no encaja.
func corregirConGuion(palabras []palabra, narracion string) ([]palabra, int, bool) {
	guion := partirPalabras(narracion)
	if len(palabras) == 0 || len(guion) == 0 {
		return palabras, 0, false
	}

	pares := alinear(palabras, guion)

	buenos := 0
	for _, p := range pares {
		if p.oido >= 0 && p.escrito >= 0 &&
			normalizar(palabras[p.oido].texto) == normalizar(guion[p.escrito]) {
			buenos++
		}
	}
	// Menos de la mitad coincidiendo no es "whisper se equivocó en algunas":
	// es que este texto no corresponde a este audio.
	if float64(buenos)/float64(len(palabras)) < 0.5 {
		return palabras, 0, false
	}

	var salida []palabra
	var pendiente []string // palabras del guion que whisper no llegó a oír
	cambios := 0

	for _, p := range pares {
		switch {
		case p.escrito < 0:
			// Whisper oyó algo que no está en el guion: no se dijo.
			cambios++
		case p.oido < 0:
			// Falta en la transcripción: se pega a la siguiente que sí tenga
			// tiempo, para que el subtítulo no se coma una palabra.
			pendiente = append(pendiente, guion[p.escrito])
		default:
			w := palabras[p.oido]
			texto := guion[p.escrito]
			if len(pendiente) > 0 {
				texto = strings.Join(append(pendiente, texto), " ")
				pendiente = nil
			}
			if texto != w.texto {
				cambios++
			}
			w.texto = texto
			salida = append(salida, w)
		}
	}
	// Lo que quedara colgando al final va con la última palabra, no se tira.
	if len(pendiente) > 0 && len(salida) > 0 {
		salida[len(salida)-1].texto += " " + strings.Join(pendiente, " ")
		cambios++
	}
	if len(salida) == 0 {
		return palabras, 0, false
	}
	return salida, cambios, true
}

type par struct{ oido, escrito int } // -1 = sin pareja

// alinear empareja las dos secuencias maximizando el parecido, al estilo
// Needleman-Wunsch. Los textos son de unas cien palabras, así que la tabla
// completa cuesta nada.
func alinear(oidas []palabra, escritas []string) []par {
	n, m := len(oidas), len(escritas)
	const hueco = -1

	tabla := make([][]int, n+1)
	for i := range tabla {
		tabla[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		tabla[i][0] = i * hueco
	}
	for j := 1; j <= m; j++ {
		tabla[0][j] = j * hueco
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			diagonal := tabla[i-1][j-1] + parecido(oidas[i-1].texto, escritas[j-1])
			arriba := tabla[i-1][j] + hueco
			izquierda := tabla[i][j-1] + hueco
			tabla[i][j] = max3(diagonal, arriba, izquierda)
		}
	}

	var pares []par
	i, j := n, m
	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 &&
			tabla[i][j] == tabla[i-1][j-1]+parecido(oidas[i-1].texto, escritas[j-1]):
			pares = append(pares, par{i - 1, j - 1})
			i, j = i-1, j-1
		case i > 0 && tabla[i][j] == tabla[i-1][j]+hueco:
			pares = append(pares, par{i - 1, -1})
			i--
		default:
			pares = append(pares, par{-1, j - 1})
			j--
		}
	}
	for a, b := 0, len(pares)-1; a < b; a, b = a+1, b-1 {
		pares[a], pares[b] = pares[b], pares[a]
	}
	return pares
}

// parecido premia la coincidencia exacta, acepta la casi-coincidencia —"rija"
// contra "hija" es un error de oído, no otra palabra— y penaliza el resto.
func parecido(a, b string) int {
	na, nb := normalizar(a), normalizar(b)
	if na == nb {
		return 2
	}
	if casiIgual(na, nb) {
		return 1
	}
	return -1
}

func casiIgual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	tope := len([]rune(a)) / 4
	if tope < 1 {
		tope = 1
	}
	return distancia(a, b) <= tope
}

// distancia es la de Levenshtein, con una sola fila en memoria.
func distancia(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	fila := make([]int, len(rb)+1)
	for j := range fila {
		fila[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		anterior := fila[0]
		fila[0] = i
		for j := 1; j <= len(rb); j++ {
			guardado := fila[j]
			coste := 1
			if ra[i-1] == rb[j-1] {
				coste = 0
			}
			fila[j] = min3(fila[j]+1, fila[j-1]+1, anterior+coste)
			anterior = guardado
		}
	}
	return fila[len(rb)]
}

// normalizar deja solo lo que importa para comparar: sin acentos, sin
// puntuación y en minúsculas. "Hija," y "hija" son la misma palabra.
func normalizar(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if plana, ok := sinAcentoSub[r]; ok {
			r = plana
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var sinAcentoSub = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
}

func partirPalabras(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	})
}

func max3(a, b, c int) int {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// narracionDe rehace el texto que se sintetizó, uniendo las escenas igual que
// se unieron para mandarlas al locutor.
func narracionDe(escenas []proveedor.EscenaRender) string {
	partes := make([]string, 0, len(escenas))
	for _, e := range escenas {
		if t := strings.TrimSpace(e.Narracion); t != "" {
			partes = append(partes, t)
		}
	}
	return strings.Join(partes, " ")
}
