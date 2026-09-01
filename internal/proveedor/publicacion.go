package proveedor

import "strings"

// EtiquetaObligatoria se añade a toda publicación, la proponga el modelo o no.
// No es un tema: es la etiqueta de alcance, y dejarla al criterio del guionista
// significaba que a veces faltaba.
const EtiquetaObligatoria = "viral"

// Etiquetas devuelve los hashtags listos para pegar —con almohadilla, sin
// repetidos y con #viral garantizado al final, que es donde van las etiquetas
// amplias: las específicas primero describen el video, la amplia lo reparte.
func (g *GuionGenerado) Etiquetas() []string {
	vistas := make(map[string]bool, len(g.Hashtags)+1)
	out := make([]string, 0, len(g.Hashtags)+1)

	agregar := func(bruto string) {
		e := normalizarEtiqueta(bruto)
		if e == "" || vistas[e] {
			return
		}
		vistas[e] = true
		out = append(out, "#"+e)
	}
	for _, h := range g.Hashtags {
		agregar(h)
	}
	agregar(EtiquetaObligatoria)
	return out
}

// normalizarEtiqueta deja una etiqueta utilizable. Un hashtag con espacios se
// corta en el primero al publicarlo, así que "salud mental" tiene que llegar
// como "saludmental" o la mitad se pierde; y los acentos, aunque se permiten,
// rompen la coincidencia con la etiqueta que ya usa todo el mundo.
func normalizarEtiqueta(s string) string {
	s = strings.TrimLeft(strings.ToLower(strings.TrimSpace(s)), "#")
	var b strings.Builder
	for _, r := range s {
		if plana, ok := sinAcento[r]; ok {
			r = plana
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var sinAcento = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c',
}

// TituloPublicable lleva solo la etiqueta obligatoria. Un título lleno de
// hashtags se trunca en pantalla y se lee como spam; el resto van en la
// descripción, que es donde hay sitio.
func (g *GuionGenerado) TituloPublicable() string {
	t := strings.TrimSpace(g.Titulo)
	if strings.Contains(strings.ToLower(t), "#"+EtiquetaObligatoria) {
		return t
	}
	return t + " #" + EtiquetaObligatoria
}

// DescripcionPublicable es el texto que se pega tal cual al publicar: la
// descripción y debajo todas las etiquetas.
func (g *GuionGenerado) DescripcionPublicable() string {
	d := strings.TrimSpace(g.Descripcion)
	etiquetas := strings.Join(g.Etiquetas(), " ")
	if d == "" {
		return etiquetas
	}
	return d + "\n\n" + etiquetas
}
