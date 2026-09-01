// Package servidor expone la cola por HTTP y sirve el panel.
//
// La interfaz compilada se empotra en el binario con go:embed. Así desplegar
// sigue siendo copiar un .exe: no hay carpeta dist/ que sincronizar ni riesgo
// de que la interfaz y la API queden en versiones distintas.
package servidor

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agente-video/internal/horario"
	"agente-video/internal/perfil"
	"agente-video/internal/temas"
	"agente-video/internal/trabajos"
)

type Servidor struct {
	cola        *trabajos.Cola
	dirPerfiles string
	interfaz    fs.FS // puede ser nil si se compiló sin panel
	banco       *temas.Banco
	horario     *horario.Horario
}

func Nuevo(cola *trabajos.Cola, dirPerfiles string, interfaz fs.FS,
	banco *temas.Banco, h *horario.Horario) *Servidor {
	return &Servidor{cola: cola, dirPerfiles: dirPerfiles, interfaz: interfaz,
		banco: banco, horario: h}
}

func (s *Servidor) Rutas() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/perfiles", s.perfiles)
	mux.HandleFunc("GET /api/perfiles/{id}", s.verPerfil)
	mux.HandleFunc("PUT /api/perfiles/{id}", s.guardarPerfil)
	mux.HandleFunc("GET /api/recursos", s.recursos)
	mux.HandleFunc("GET /api/trabajos", s.listar)
	mux.HandleFunc("POST /api/trabajos", s.encolar)
	mux.HandleFunc("DELETE /api/trabajos/{id}", s.olvidar)
	mux.HandleFunc("POST /api/trabajos/{id}/cancelar", s.cancelar)
	mux.HandleFunc("GET /api/temas", s.listarTemas)
	mux.HandleFunc("POST /api/temas", s.agregarTemas)
	mux.HandleFunc("PATCH /api/temas/{id}", s.cambiarTema)
	mux.HandleFunc("DELETE /api/temas/{id}", s.olvidarTema)
	mux.HandleFunc("GET /api/horario", s.listarReglas)
	mux.HandleFunc("POST /api/horario", s.guardarRegla)
	mux.HandleFunc("DELETE /api/horario/{id}", s.olvidarRegla)
	mux.HandleFunc("GET /api/eventos", s.eventos)
	mux.HandleFunc("GET /media/{id}/video", s.video)
	mux.HandleFunc("GET /media/{id}/textos", s.textos)

	if s.interfaz != nil {
		mux.Handle("/", s.panel())
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "el panel no se incluyó en esta compilación; "+
				"construye la interfaz con: cd interfaz && npm install && npm run build",
				http.StatusNotFound)
		})
	}
	return registrar(mux)
}

// --- API ---

func (s *Servidor) perfiles(w http.ResponseWriter, r *http.Request) {
	ids, err := perfil.Listar(s.dirPerfiles)
	if err != nil {
		fallo(w, http.StatusInternalServerError, err)
		return
	}
	type resumen struct {
		ID       string `json:"id"`
		Nombre   string `json:"nombre"`
		Escenas  int    `json:"escenas"`
		Segundos int    `json:"segundos"`
		Formato  string `json:"formato"`
	}
	out := []resumen{}
	for _, id := range ids {
		p, err := perfil.Cargar(s.dirPerfiles, id)
		if err != nil {
			continue // un perfil roto no debe tumbar el panel entero
		}
		out = append(out, resumen{
			ID: p.ID, Nombre: p.Nombre,
			Escenas: p.Guion.Escenas, Segundos: p.Guion.DuracionSeg,
			Formato: fmt.Sprintf("%dx%d", p.Formato.Ancho, p.Formato.Alto),
		})
	}
	responder(w, out)
}

func (s *Servidor) listar(w http.ResponseWriter, r *http.Request) {
	responder(w, s.cola.Listar())
}

func (s *Servidor) encolar(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Perfil string   `json:"perfil"`
		Tema   string   `json:"tema"`
		Temas  []string `json:"temas"` // encolar varios de una vez
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&cuerpo); err != nil {
		fallo(w, http.StatusBadRequest, fmt.Errorf("cuerpo inválido: %w", err))
		return
	}

	temas := cuerpo.Temas
	if cuerpo.Tema != "" {
		temas = append(temas, cuerpo.Tema)
	}
	if len(temas) == 0 {
		fallo(w, http.StatusBadRequest, fmt.Errorf("hace falta al menos un tema"))
		return
	}

	var creados []*trabajos.Trabajo
	for _, tema := range temas {
		tema = strings.TrimSpace(tema)
		if tema == "" {
			continue
		}
		t, err := s.cola.Encolar(cuerpo.Perfil, tema)
		if err != nil {
			fallo(w, http.StatusBadRequest, err)
			return
		}
		creados = append(creados, t)
	}
	w.WriteHeader(http.StatusCreated)
	responder(w, creados)
}

func (s *Servidor) cancelar(w http.ResponseWriter, r *http.Request) {
	if err := s.cola.Cancelar(r.PathValue("id")); err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	responder(w, map[string]string{"estado": "ok"})
}

func (s *Servidor) olvidar(w http.ResponseWriter, r *http.Request) {
	if err := s.cola.Olvidar(r.PathValue("id")); err != nil {
		fallo(w, http.StatusBadRequest, err)
		return
	}
	responder(w, map[string]string{"estado": "ok"})
}

// eventos mantiene abierta una conexión y empuja cada cambio de estado.
// Se eligió SSE sobre WebSocket porque el flujo es en un solo sentido y SSE
// reconecta solo, sin código de reconexión en el cliente.
func (s *Servidor) eventos(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming no soportado", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, cerrar := s.cola.Suscribir()
	defer cerrar()

	// Estado completo al conectar, para que la interfaz no tenga que pedirlo
	// aparte y arranque ya sincronizada.
	escribirEvento(w, flusher, "lista", s.cola.Listar())

	// Latido: sin tráfico, los intermediarios cierran la conexión en silencio.
	latido := time.NewTicker(20 * time.Second)
	defer latido.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-latido.C:
			fmt.Fprint(w, ": latido\n\n")
			flusher.Flush()
		case e, abierto := <-ch:
			if !abierto {
				return
			}
			if e.Tipo == "lista" {
				escribirEvento(w, flusher, "lista", s.cola.Listar())
			} else {
				escribirEvento(w, flusher, "estado", e.Trabajo)
			}
		}
	}
}

func escribirEvento(w http.ResponseWriter, f http.Flusher, tipo string, datos any) {
	b, err := json.Marshal(datos)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", tipo, b)
	f.Flush()
}

// --- medios ---

func (s *Servidor) video(w http.ResponseWriter, r *http.Request) {
	t := s.cola.Uno(r.PathValue("id"))
	if t == nil || t.Video == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(t.Video)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	// ServeContent da soporte de rangos, que es lo que permite adelantar el
	// video en el reproductor sin descargarlo entero.
	http.ServeContent(w, r, filepath.Base(t.Video), info.ModTime(), f)
}

func (s *Servidor) textos(w http.ResponseWriter, r *http.Request) {
	t := s.cola.Uno(r.PathValue("id"))
	if t == nil || t.Textos == "" {
		http.NotFound(w, r)
		return
	}
	datos, err := os.ReadFile(t.Textos)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(datos)
}

// --- panel ---

// panel sirve la SPA. Cualquier ruta desconocida devuelve index.html para que
// el enrutado del cliente funcione al recargar.
func (s *Servidor) panel() http.Handler {
	archivos := http.FileServer(http.FS(s.interfaz))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := strings.TrimPrefix(r.URL.Path, "/")
		if ruta == "" {
			ruta = "index.html"
		}
		if _, err := fs.Stat(s.interfaz, ruta); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		archivos.ServeHTTP(w, r)
	})
}

// --- utilidades ---

func responder(w http.ResponseWriter, datos any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(datos); err != nil {
		log.Printf("error escribiendo respuesta: %v", err)
	}
}

func fallo(w http.ResponseWriter, codigo int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func registrar(siguiente http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// El flujo de eventos vive minutos: registrarlo al terminar no sirve.
		if strings.HasPrefix(r.URL.Path, "/api/eventos") {
			siguiente.ServeHTTP(w, r)
			return
		}
		inicio := time.Now()
		siguiente.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(inicio).Round(time.Millisecond))
		}
	})
}
