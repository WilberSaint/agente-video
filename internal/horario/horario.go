// Package horario dispara la generación sin que nadie esté delante.
//
// Es la última pieza que faltaba para que el agente produzca solo: el banco
// guarda las ideas y esto decide cuándo convertirlas en video.
//
// Se comprueba cada minuto en lugar de calcular el siguiente disparo y dormir
// hasta entonces. Es más simple y, sobre todo, sobrevive a que el servidor se
// reinicie, a que la máquina hiberne o a que alguien cambie la hora: un cálculo
// hecho una vez se queda desfasado, una comprobación periódica no.
package horario

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Regla struct {
	ID     string `json:"id"`
	Activa bool   `json:"activa"`
	Perfil string `json:"perfil"`
	// Hora en formato 15:04, hora local de la máquina.
	Hora string `json:"hora"`
	// Dias de la semana en que aplica, 0=domingo. Vacío = todos los días.
	Dias []int `json:"dias"`
	// Cantidad de videos a encolar en cada disparo.
	Cantidad int `json:"cantidad"`
	// ProponerSiFaltan pide ideas nuevas cuando el banco no da para la tanda.
	// Sin esto, un banco vacío significa una noche perdida.
	ProponerSiFaltan bool `json:"proponer_si_faltan"`

	UltimoDisparo *time.Time `json:"ultimo_disparo,omitempty"`
	UltimoResumen string     `json:"ultimo_resumen,omitempty"`
}

// Disparar encola una tanda. Devuelve qué pasó, para dejarlo escrito en la
// regla: al llegar por la mañana, lo primero que se quiere saber es si trabajó.
type Disparar func(ctx context.Context, r Regla) string

type Horario struct {
	mu       sync.RWMutex
	ruta     string
	reglas   []*Regla
	disparar Disparar
	hecho    chan struct{}
}

func Nuevo(ruta string, disparar Disparar) *Horario {
	h := &Horario{ruta: ruta, disparar: disparar}
	h.cargar()
	return h
}

func (h *Horario) Arrancar(ctx context.Context) {
	h.hecho = make(chan struct{})
	go h.vigilar(ctx)
}

func (h *Horario) Esperar(plazo time.Duration) {
	if h.hecho == nil {
		return
	}
	select {
	case <-h.hecho:
	case <-time.After(plazo):
	}
}

func (h *Horario) vigilar(ctx context.Context) {
	defer close(h.hecho)
	// Se comprueba al arrancar y luego cada minuto. La primera comprobación
	// importa: si el servidor estaba caído a la hora prevista, al volver dentro
	// del mismo minuto todavía se puede recuperar el disparo.
	h.comprobar(ctx)

	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.comprobar(ctx)
		}
	}
}

func (h *Horario) comprobar(ctx context.Context) {
	ahora := time.Now()
	for _, r := range h.pendientesDeDisparar(ahora) {
		resumen := h.disparar(ctx, *r)

		h.mu.Lock()
		for _, x := range h.reglas {
			if x.ID == r.ID {
				t := ahora
				x.UltimoDisparo = &t
				x.UltimoResumen = resumen
				break
			}
		}
		h.mu.Unlock()
		h.guardar()
	}
}

// pendientesDeDisparar devuelve las reglas que toca ejecutar ahora.
func (h *Horario) pendientesDeDisparar(ahora time.Time) []*Regla {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var listas []*Regla
	for _, r := range h.reglas {
		if !r.Activa || r.Cantidad <= 0 {
			continue
		}
		if !aplicaHoy(r, ahora) {
			continue
		}
		hh, mm, err := partirHora(r.Hora)
		if err != nil {
			continue
		}
		previsto := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), hh, mm, 0, 0, ahora.Location())
		if ahora.Before(previsto) {
			continue
		}
		// Ventana de gracia: si el servidor estuvo caído a la hora exacta, aún
		// se recupera el disparo dentro de la hora siguiente. Pasado eso se deja
		// pasar el día: producir a destiempo es peor que no producir.
		if ahora.Sub(previsto) > time.Hour {
			continue
		}
		// Una vez al día: comparar por fecha y no por intervalo evita disparar
		// dos veces si la máquina cambia de hora o el reloj salta.
		if r.UltimoDisparo != nil && mismoDia(*r.UltimoDisparo, ahora) {
			continue
		}
		listas = append(listas, r)
	}
	return listas
}

func aplicaHoy(r *Regla, ahora time.Time) bool {
	if len(r.Dias) == 0 {
		return true
	}
	hoy := int(ahora.Weekday())
	for _, d := range r.Dias {
		if d == hoy {
			return true
		}
	}
	return false
}

func mismoDia(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func partirHora(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("hora inválida %q, se espera HH:MM", s)
	}
	return t.Hour(), t.Minute(), nil
}

// --- gestión ---

func (h *Horario) Listar() []*Regla {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Regla, 0, len(h.reglas))
	for _, r := range h.reglas {
		c := *r
		out = append(out, &c)
	}
	return out
}

func (h *Horario) Guardar(r Regla) (*Regla, error) {
	if _, _, err := partirHora(r.Hora); err != nil {
		return nil, err
	}
	if r.Perfil == "" {
		return nil, fmt.Errorf("hace falta un perfil")
	}
	if r.Cantidad <= 0 {
		return nil, fmt.Errorf("la cantidad debe ser al menos 1")
	}

	h.mu.Lock()
	if r.ID == "" {
		r.ID = fmt.Sprintf("r%d", time.Now().UnixNano())
		h.reglas = append(h.reglas, &r)
	} else {
		encontrada := false
		for i, x := range h.reglas {
			if x.ID == r.ID {
				// El histórico no viaja en el formulario; se conserva.
				r.UltimoDisparo = x.UltimoDisparo
				r.UltimoResumen = x.UltimoResumen
				h.reglas[i] = &r
				encontrada = true
				break
			}
		}
		if !encontrada {
			h.reglas = append(h.reglas, &r)
		}
	}
	h.mu.Unlock()

	h.guardar()
	copia := r
	return &copia, nil
}

func (h *Horario) Olvidar(id string) {
	h.mu.Lock()
	for i, r := range h.reglas {
		if r.ID == id {
			h.reglas = append(h.reglas[:i], h.reglas[i+1:]...)
			break
		}
	}
	h.mu.Unlock()
	h.guardar()
}

// --- persistencia ---

func (h *Horario) guardar() {
	if h.ruta == "" {
		return
	}
	h.mu.RLock()
	datos, err := json.MarshalIndent(h.reglas, "", "  ")
	h.mu.RUnlock()
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Dir(h.ruta)); err != nil {
		return
	}
	tmp := h.ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, h.ruta) != nil {
		_ = os.Remove(tmp)
	}
}

func (h *Horario) cargar() {
	datos, err := os.ReadFile(h.ruta)
	if err != nil {
		return
	}
	_ = json.Unmarshal(datos, &h.reglas)
}
