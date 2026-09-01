package voz

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agente-video/internal/proveedor"
)

// Control de consumo de un proveedor de voz de pago.
//
// ElevenLabs cobra por carácter y la cuota se agota sin avisar: la petición
// número cincuenta y uno falla, y si eso ocurre a las tres de la mañana con
// diez videos en cola, se pierde la noche entera.
//
// Aquí se lleva la cuenta y, al llegar al límite, se pasa solo al proveedor de
// respaldo. El video sale con peor voz, pero sale. Perder calidad en un video
// es preferible a perder el lote.

// Contador persiste el consumo por mes natural.
type Contador struct {
	mu    sync.Mutex
	ruta  string
	datos consumo
}

type consumo struct {
	// Mes en formato 2006-01. Al cambiar, el contador se reinicia solo: las
	// cuotas de ElevenLabs son mensuales.
	Mes        string `json:"mes"`
	Caracteres int    `json:"caracteres"`
	Peticiones int    `json:"peticiones"`
	UltimoUso  string `json:"ultimo_uso"`
}

func NuevoContador(ruta string) *Contador {
	c := &Contador{ruta: ruta}
	c.cargar()
	return c
}

func (c *Contador) cargar() {
	datos, err := os.ReadFile(c.ruta)
	if err != nil {
		return
	}
	_ = json.Unmarshal(datos, &c.datos)
	if c.datos.Mes != mesActual() {
		c.datos = consumo{Mes: mesActual()}
	}
}

func (c *Contador) guardar() {
	_ = os.MkdirAll(filepath.Dir(c.ruta), 0o755)
	datos, err := json.MarshalIndent(c.datos, "", "  ")
	if err != nil {
		return
	}
	tmp := c.ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, c.ruta) != nil {
		_ = os.Remove(tmp)
	}
}

func mesActual() string { return time.Now().Format("2006-01") }

// Cabe indica si quedan caracteres suficientes para un texto. limite en 0
// significa sin control.
func (c *Contador) Cabe(caracteres, limite int) bool {
	if limite <= 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.datos.Mes != mesActual() {
		c.datos = consumo{Mes: mesActual()}
	}
	return c.datos.Caracteres+caracteres <= limite
}

// Anotar registra un consumo ya realizado.
func (c *Contador) Anotar(caracteres int) {
	c.mu.Lock()
	if c.datos.Mes != mesActual() {
		c.datos = consumo{Mes: mesActual()}
	}
	c.datos.Caracteres += caracteres
	c.datos.Peticiones++
	c.datos.UltimoUso = time.Now().Format(time.RFC3339)
	c.mu.Unlock()
	c.guardar()
}

// Estado devuelve el consumo del mes en curso.
func (c *Contador) Estado() (caracteres, peticiones int, mes string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.datos.Mes != mesActual() {
		return 0, 0, mesActual()
	}
	return c.datos.Caracteres, c.datos.Peticiones, c.datos.Mes
}

// ConRespaldo envuelve un locutor de pago y cae a otro cuando se agota la cuota.
type ConRespaldo struct {
	Principal proveedor.Locutor
	Respaldo  proveedor.Locutor
	Contador  *Contador
	Limite    int
	// ModeloRespaldo es el modelo que necesita el suplente. Hace falta porque
	// voz.modelo del perfil describe la voz del proveedor PRINCIPAL: con
	// ElevenLabs es un identificador de voz, y Piper esperaría la ruta de un
	// .onnx. Sin esto el respaldo falla justo cuando más se le necesita.
	ModeloRespaldo string
	// Aviso comunica el cambio de proveedor. Debe ser visible: el video sale
	// con otra voz, y descubrirlo al reproducirlo desconcierta.
	Aviso func(string, ...any)
}

// paraRespaldo ajusta la petición al suplente.
func (c *ConRespaldo) paraRespaldo(req proveedor.PeticionVoz) proveedor.PeticionVoz {
	if c.ModeloRespaldo != "" {
		req.Modelo = c.ModeloRespaldo
	}
	return req
}

func (c *ConRespaldo) avisar(f string, a ...any) {
	if c.Aviso != nil {
		c.Aviso(f, a...)
	}
}

func (c *ConRespaldo) Nombre() string {
	return c.Principal.Nombre() + " (respaldo: " + c.Respaldo.Nombre() + ")"
}

func (c *ConRespaldo) Sintetizar(ctx context.Context, req proveedor.PeticionVoz) error {
	n := len([]rune(req.Texto))

	if !c.Contador.Cabe(n, c.Limite) {
		usados, _, mes := c.Contador.Estado()
		c.avisar("cuota de voz agotada este mes (%d de %d caracteres en %s). "+
			"Se usa %s. Renueva el plan o cambia la llave para volver a la voz buena.",
			usados, c.Limite, mes, c.Respaldo.Nombre())
		return c.Respaldo.Sintetizar(ctx, c.paraRespaldo(req))
	}

	err := c.Principal.Sintetizar(ctx, req)
	if err == nil {
		c.Contador.Anotar(n)
		usados, _, _ := c.Contador.Estado()
		if c.Limite > 0 {
			restante := c.Limite - usados
			// Avisar cerca del final da margen para reaccionar antes de que un
			// lote nocturno se quede sin voz a mitad.
			if restante < c.Limite/10 {
				c.avisar("quedan %d caracteres de voz este mes (~%d videos)",
					restante, restante/600)
			}
		}
		return nil
	}

	// Un fallo de cuota o de credencial no se arregla reintentando; se cambia
	// de proveedor y el lote continúa.
	if proveedor.EsPermanente(err) || esCuotaAgotada(err) {
		c.avisar("la voz principal falló (%v). Se usa %s para no detener el lote.",
			err, c.Respaldo.Nombre())
		return c.Respaldo.Sintetizar(ctx, c.paraRespaldo(req))
	}
	return err
}

func esCuotaAgotada(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marca := range []string{"quota", "401", "429"} {
		if strings.Contains(strings.ToLower(msg), marca) {
			return true
		}
	}
	return false
}

var _ proveedor.Locutor = (*ConRespaldo)(nil)
