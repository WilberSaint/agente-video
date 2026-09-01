package servidor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agente-video/internal/perfil"
)

// Edición de perfiles desde el panel.
//
// Hasta aquí había que abrir el JSON a mano para cambiar la voz, el personaje o
// el proveedor de imágenes. Con dos proveedores entre los que alternar y varios
// canales, eso es justo la fricción que la interfaz venía a quitar.
//
// El editor guarda el perfil completo, no un parche: el archivo se valida antes
// de escribirse, así que un perfil roto se rechaza en vez de dejar el agente sin
// poder arrancar.

// verPerfil devuelve el perfil tal cual, con los valores por defecto ya
// aplicados: la interfaz debe mostrar lo que el agente va a usar de verdad, no
// los huecos que el archivo dejó sin rellenar.
func (s *Servidor) verPerfil(w http.ResponseWriter, r *http.Request) {
	p, err := perfil.Cargar(s.dirPerfiles, r.PathValue("id"))
	if err != nil {
		fallo(w, http.StatusNotFound, err)
		return
	}
	responder(w, p)
}

func (s *Servidor) guardarPerfil(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ruta := filepath.Join(s.dirPerfiles, id, "perfil.json")
	if _, err := os.Stat(ruta); err != nil {
		fallo(w, http.StatusNotFound, fmt.Errorf("no existe el perfil %q", id))
		return
	}

	var p perfil.Perfil
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		fallo(w, http.StatusBadRequest, fmt.Errorf("perfil inválido: %w", err))
		return
	}
	// El id lo manda la ruta, no el cuerpo: si no, renombrar desde la interfaz
	// escribiría en la carpeta equivocada.
	p.ID = id

	if err := p.Validar(); err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}

	datos, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		fallo(w, http.StatusInternalServerError, err)
		return
	}

	// Escritura atómica: un corte a media escritura dejaría el perfil ilegible
	// y el agente no podría ni arrancar ese canal.
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.Rename(tmp, ruta); err != nil {
		_ = os.Remove(tmp)
		fallo(w, http.StatusInternalServerError, err)
		return
	}

	// Se devuelve lo recargado del disco, con los valores por defecto ya
	// aplicados, para que la interfaz refleje el estado real tras guardar.
	recargado, err := perfil.Cargar(s.dirPerfiles, id)
	if err != nil {
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	responder(w, recargado)
}

// Recursos son las opciones que el editor puede ofrecer en desplegables. Sin
// esto habría que escribir rutas a mano, que es la mitad de los errores.
type Recursos struct {
	Voces      []opcion `json:"voces"`
	Musica     []opcion `json:"musica"`
	Efectos    []opcion `json:"efectos"`
	Personajes []opcion `json:"personajes"`
}

type opcion struct {
	// Valor es lo que va al perfil; Etiqueta lo que ve la persona.
	Valor    string `json:"valor"`
	Etiqueta string `json:"etiqueta"`
	Nota     string `json:"nota,omitempty"`
}

func (s *Servidor) recursos(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("perfil")
	raiz := filepath.Join(s.dirPerfiles, id)

	res := Recursos{
		Voces:      buscarArchivos("bin/voces", raiz, ".onnx", ""),
		Musica:     buscarArchivos("assets/musica", raiz, ".mp3", ".wav"),
		Efectos:    buscarArchivos("assets/efectos", raiz, ".wav", ".mp3"),
		Personajes: carpetasDePersonaje(raiz),
	}
	responder(w, res)
}

// buscarArchivos lista los archivos de una carpeta como rutas relativas al
// perfil, que es como las espera el JSON.
func buscarArchivos(dir, raizPerfil string, exts ...string) []opcion {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return []opcion{}
	}
	out := []opcion{}
	for _, e := range entradas {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !coincideExt(ext, exts) {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		rel := relativaA(raizPerfil, abs)
		out = append(out, opcion{
			Valor:    rel,
			Etiqueta: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Etiqueta < out[j].Etiqueta })
	return out
}

func coincideExt(ext string, exts []string) bool {
	for _, e := range exts {
		if e != "" && ext == e {
			return true
		}
	}
	return false
}

// carpetasDePersonaje ofrece tanto la carpeta raíz —para que roten todas las
// expresiones— como cada subcarpeta, por si hay varios personajes.
func carpetasDePersonaje(raizPerfil string) []opcion {
	base := filepath.Join(raizPerfil, "personaje")
	entradas, err := os.ReadDir(base)
	if err != nil {
		return []opcion{}
	}

	out := []opcion{{Valor: "", Etiqueta: "(sin personaje)"}}
	imagenes := 0
	for _, e := range entradas {
		if e.IsDir() {
			if n := contarImagenes(filepath.Join(base, e.Name())); n > 0 {
				out = append(out, opcion{
					Valor:    "personaje/" + e.Name() + "/",
					Etiqueta: e.Name(),
					Nota:     fmt.Sprintf("%d expresiones", n),
				})
			}
			continue
		}
		if esImagen(e.Name()) {
			imagenes++
		}
	}
	if imagenes > 0 {
		out = append(out, opcion{
			Valor:    "personaje/",
			Etiqueta: "personaje (raíz)",
			Nota:     fmt.Sprintf("%d expresiones", imagenes),
		})
	}
	return out
}

func contarImagenes(dir string) int {
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entradas {
		if !e.IsDir() && esImagen(e.Name()) {
			n++
		}
	}
	return n
}

func esImagen(nombre string) bool {
	switch strings.ToLower(filepath.Ext(nombre)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}

// relativaA calcula la ruta desde la carpeta del perfil, con barras normales.
// El perfil guarda rutas relativas para que mover el proyecto no rompa nada.
func relativaA(raizPerfil, destino string) string {
	absRaiz, err := filepath.Abs(raizPerfil)
	if err != nil {
		return destino
	}
	rel, err := filepath.Rel(absRaiz, destino)
	if err != nil {
		return destino
	}
	return filepath.ToSlash(rel)
}
