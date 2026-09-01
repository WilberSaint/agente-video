// Package temas guarda las ideas pendientes de convertirse en video.
//
// Es la pieza que separa "una herramienta que usas" de "un agente que produce":
// sin un banco, alguien tiene que estar delante escribiendo el tema. Con él, el
// horario puede tomar el siguiente pendiente a las tres de la mañana.
package temas

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Estado string

const (
	Pendiente  Estado = "pendiente"
	Usado      Estado = "usado"
	Descartado Estado = "descartado"
)

type Tema struct {
	ID     string `json:"id"`
	Texto  string `json:"texto"`
	Perfil string `json:"perfil"` // vacío = sirve para cualquiera
	Estado Estado `json:"estado"`
	// Origen distingue lo que escribiste tú de lo que propuso el agente. Importa
	// para saber si las ideas propias se están agotando.
	Origen string `json:"origen"` // manual | propuesto

	Creado    time.Time  `json:"creado"`
	Usado     *time.Time `json:"usado,omitempty"`
	TrabajoID string     `json:"trabajo_id,omitempty"`
}

type Banco struct {
	mu    sync.RWMutex
	ruta  string
	temas []*Tema
}

func Nuevo(ruta string) *Banco {
	b := &Banco{ruta: ruta}
	b.cargar()
	return b
}

// Agregar añade temas nuevos. Devuelve cuántos entraron: los repetidos se
// ignoran en silencio, porque pegar dos veces la misma lista es lo normal y
// duplicar temas produciría dos videos idénticos.
func (b *Banco) Agregar(textos []string, perfil, origen string) int {
	if origen == "" {
		origen = "manual"
	}
	b.mu.Lock()
	existentes := map[string]bool{}
	for _, t := range b.temas {
		if t.Estado != Descartado {
			existentes[normalizar(t.Texto)] = true
		}
	}

	nuevos := 0
	for _, texto := range textos {
		texto = strings.TrimSpace(texto)
		if texto == "" || existentes[normalizar(texto)] {
			continue
		}
		existentes[normalizar(texto)] = true
		b.temas = append(b.temas, &Tema{
			ID:     nuevoID(),
			Texto:  texto,
			Perfil: perfil,
			Estado: Pendiente,
			Origen: origen,
			Creado: time.Now(),
		})
		nuevos++
	}
	b.mu.Unlock()

	if nuevos > 0 {
		b.guardar()
	}
	return nuevos
}

// Tomar reserva hasta n temas pendientes del perfil y los marca como usados.
//
// Marcar y devolver en la misma operación es deliberado: si se consultara
// primero y se marcara después, dos disparos del horario a la vez tomarían los
// mismos temas y generarían videos duplicados.
func (b *Banco) Tomar(perfil string, n int) []*Tema {
	b.mu.Lock()
	defer b.mu.Unlock()

	ahora := time.Now()
	var tomados []*Tema
	for _, t := range b.temas {
		if len(tomados) >= n {
			break
		}
		if t.Estado != Pendiente {
			continue
		}
		// Un tema sin perfil sirve para cualquiera; con perfil, solo para el suyo.
		if t.Perfil != "" && t.Perfil != perfil {
			continue
		}
		t.Estado = Usado
		t.Usado = &ahora
		tomados = append(tomados, t.copia())
	}
	if len(tomados) > 0 {
		go b.guardar()
	}
	return tomados
}

// MarcarTrabajo asocia un tema con el trabajo que lo consumió, para poder
// rastrear qué video salió de qué idea.
func (b *Banco) MarcarTrabajo(idTema, idTrabajo string) {
	b.mu.Lock()
	for _, t := range b.temas {
		if t.ID == idTema {
			t.TrabajoID = idTrabajo
			break
		}
	}
	b.mu.Unlock()
	b.guardar()
}

// Devolver vuelve a poner un tema como pendiente. Se usa cuando el trabajo que
// lo consumió falló: la idea sigue siendo buena, lo que falló fue la máquina.
func (b *Banco) Devolver(id string) {
	b.mu.Lock()
	for _, t := range b.temas {
		if t.ID == id {
			t.Estado = Pendiente
			t.Usado = nil
			t.TrabajoID = ""
			break
		}
	}
	b.mu.Unlock()
	b.guardar()
}

func (b *Banco) CambiarEstado(id string, e Estado) error {
	b.mu.Lock()
	var encontrado bool
	for _, t := range b.temas {
		if t.ID == id {
			t.Estado = e
			encontrado = true
			break
		}
	}
	b.mu.Unlock()
	if !encontrado {
		return fmt.Errorf("no existe el tema %s", id)
	}
	b.guardar()
	return nil
}

func (b *Banco) Olvidar(id string) {
	b.mu.Lock()
	for i, t := range b.temas {
		if t.ID == id {
			b.temas = append(b.temas[:i], b.temas[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
	b.guardar()
}

// Listar devuelve copias, con los pendientes primero: es lo que interesa ver.
func (b *Banco) Listar(perfil string) []*Tema {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := []*Tema{}
	for _, t := range b.temas {
		if perfil != "" && t.Perfil != "" && t.Perfil != perfil {
			continue
		}
		out = append(out, t.copia())
	}
	// Orden estable: pendientes arriba, y dentro de cada grupo lo más reciente
	// primero, que es lo que se acaba de escribir.
	orden := map[Estado]int{Pendiente: 0, Usado: 1, Descartado: 2}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			a, c := out[j-1], out[j]
			if orden[a.Estado] < orden[c.Estado] {
				break
			}
			if orden[a.Estado] == orden[c.Estado] && a.Creado.After(c.Creado) {
				break
			}
			out[j-1], out[j] = c, a
		}
	}
	return out
}

// Pendientes cuenta los disponibles para un perfil. El horario lo usa para
// decidir si hace falta proponer más antes de quedarse sin nada que producir.
func (b *Banco) Pendientes(perfil string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := 0
	for _, t := range b.temas {
		if t.Estado != Pendiente {
			continue
		}
		if t.Perfil != "" && t.Perfil != perfil {
			continue
		}
		n++
	}
	return n
}

// UsadosRecientes devuelve los textos ya producidos, para que quien proponga
// ideas nuevas no repita las de siempre.
func (b *Banco) UsadosRecientes(perfil string, max int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out []string
	for i := len(b.temas) - 1; i >= 0 && len(out) < max; i-- {
		t := b.temas[i]
		if t.Estado == Pendiente {
			continue
		}
		if t.Perfil != "" && perfil != "" && t.Perfil != perfil {
			continue
		}
		out = append(out, t.Texto)
	}
	return out
}

// --- persistencia ---

func (b *Banco) guardar() {
	if b.ruta == "" {
		return
	}
	b.mu.RLock()
	datos, err := json.MarshalIndent(b.temas, "", "  ")
	b.mu.RUnlock()
	if err != nil {
		return
	}
	if _, err := os.Stat(filepath.Dir(b.ruta)); err != nil {
		return
	}
	tmp := b.ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, b.ruta) != nil {
		_ = os.Remove(tmp)
	}
}

func (b *Banco) cargar() {
	datos, err := os.ReadFile(b.ruta)
	if err != nil {
		return
	}
	_ = json.Unmarshal(datos, &b.temas)
}

func (t *Tema) copia() *Tema {
	c := *t
	return &c
}

// normalizar reduce un texto a su forma comparable, para detectar repetidos
// escritos con distinta puntuación o mayúsculas.
func normalizar(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var secuencia atomic.Uint64

func nuevoID() string {
	const letras = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("t%d", secuencia.Add(1))
	}
	for i := range b {
		b[i] = letras[int(b[i])%len(letras)]
	}
	return string(b)
}
