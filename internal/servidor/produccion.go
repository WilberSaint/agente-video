package servidor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"agente-video/internal/horario"
	"agente-video/internal/temas"
)

// Banco y horario por HTTP.
//
// Todo se maneja desde el panel: escribir ideas en un archivo o editar un cron
// a mano es exactamente lo que la interfaz venía a evitar.

func (s *Servidor) listarTemas(w http.ResponseWriter, r *http.Request) {
	if s.banco == nil {
		responder(w, []any{})
		return
	}
	responder(w, s.banco.Listar(r.URL.Query().Get("perfil")))
}

func (s *Servidor) agregarTemas(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Perfil string   `json:"perfil"`
		Temas  []string `json:"temas"`
		Texto  string   `json:"texto"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	lista := cuerpo.Temas
	if cuerpo.Texto != "" {
		// El panel manda un bloque de texto: una idea por línea, igual que en
		// el lanzador, para poder pegar varias de golpe.
		lista = append(lista, strings.Split(cuerpo.Texto, "\n")...)
	}

	nuevos := s.banco.Agregar(lista, cuerpo.Perfil, "manual")
	w.WriteHeader(http.StatusCreated)
	responder(w, map[string]any{
		"nuevos": nuevos,
		// Se informa de los repetidos porque el silencio haría pensar que se
		// perdieron: pegar dos veces la misma lista es de lo más normal.
		"repetidos": contarNoVacias(lista) - nuevos,
		"temas":     s.banco.Listar(cuerpo.Perfil),
	})
}

func (s *Servidor) cambiarTema(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Estado string `json:"estado"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	e := temas.Estado(cuerpo.Estado)
	switch e {
	case temas.Pendiente, temas.Usado, temas.Descartado:
	default:
		fallo(w, http.StatusBadRequest, fmt.Errorf("estado desconocido %q", cuerpo.Estado))
		return
	}
	if err := s.banco.CambiarEstado(r.PathValue("id"), e); err != nil {
		fallo(w, http.StatusNotFound, err)
		return
	}
	responder(w, map[string]string{"estado": "ok"})
}

func (s *Servidor) olvidarTema(w http.ResponseWriter, r *http.Request) {
	s.banco.Olvidar(r.PathValue("id"))
	responder(w, map[string]string{"estado": "ok"})
}

// --- horario ---

func (s *Servidor) listarReglas(w http.ResponseWriter, r *http.Request) {
	if s.horario == nil {
		responder(w, []any{})
		return
	}
	responder(w, s.horario.Listar())
}

func (s *Servidor) guardarRegla(w http.ResponseWriter, r *http.Request) {
	var regla horario.Regla
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&regla); err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	guardada, err := s.horario.Guardar(regla)
	if err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	responder(w, guardada)
}

func (s *Servidor) olvidarRegla(w http.ResponseWriter, r *http.Request) {
	s.horario.Olvidar(r.PathValue("id"))
	responder(w, map[string]string{"estado": "ok"})
}

func contarNoVacias(ss []string) int {
	n := 0
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			n++
		}
	}
	return n
}
