<script setup>
import { ref, watch, computed } from 'vue'

const props = defineProps({ perfilId: { type: String, required: true } })
const emit = defineEmits(['cerrar', 'guardado'])

const p = ref(null)
const recursos = ref({ voces: [], musica: [], efectos: [], personajes: [] })
const cargando = ref(true)
const guardando = ref(false)
const error = ref('')
const guardado = ref(false)

async function cargar() {
  cargando.value = true
  error.value = ''
  try {
    const [rp, rr] = await Promise.all([
      fetch(`/api/perfiles/${props.perfilId}`),
      fetch(`/api/recursos?perfil=${props.perfilId}`),
    ])
    if (!rp.ok) throw new Error((await rp.json()).error || 'no se pudo cargar')
    p.value = await rp.json()
    recursos.value = await rr.json()
  } catch (e) {
    error.value = e.message
  } finally {
    cargando.value = false
  }
}

async function guardar() {
  guardando.value = true
  error.value = ''
  guardado.value = false
  try {
    const r = await fetch(`/api/perfiles/${props.perfilId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(p.value),
    })
    if (!r.ok) throw new Error((await r.json()).error || 'no se pudo guardar')
    p.value = await r.json()
    guardado.value = true
    emit('guardado')
    setTimeout(() => (guardado.value = false), 2500)
  } catch (e) {
    error.value = e.message
  } finally {
    guardando.value = false
  }
}

watch(() => props.perfilId, cargar, { immediate: true })

// Avisos que el propio agente daría al generar, pero que aquí se ven antes de
// gastar diez minutos en un video.
const avisos = computed(() => {
  if (!p.value) return []
  const a = []
  if (p.value.imagen.proveedor === 'cloudflare') {
    a.push('Cloudflare no admite semilla fija: los personajes y lugares que se ' +
           'repitan entre planos pueden no verse iguales. Para historias con ' +
           'personajes recurrentes, usa Pollinations.')
  }
  if (p.value.voz.proveedor === 'elevenlabs' && !p.value.voz.limite_caracteres) {
    a.push('Sin límite de caracteres no hay respaldo: si se agota la cuota a ' +
           'mitad de un lote, el resto falla en vez de seguir con Piper.')
  }
  if (p.value.personaje.imagen && p.value.subtitulos.activos) {
    const alto = (p.value.formato.alto * (p.value.personaje.alto_pct || 30)) / 100
    const necesario = (p.value.personaje.margen || 60) + alto + 40
    if (necesario > p.value.subtitulos.margen_v) {
      a.push(`Los subtítulos subirán solos a ${Math.round(necesario)}px para no ` +
             `quedar bajo el personaje.`)
    }
  }
  return a
})
</script>

<template>
  <div class="fondo" @click.self="$emit('cerrar')">
    <div class="panel-editor">
      <header>
        <h2>Editar perfil<span v-if="p"> — {{ p.nombre }}</span></h2>
        <button class="tenue" @click="$emit('cerrar')">cerrar</button>
      </header>

      <p v-if="cargando" class="silencio">Cargando…</p>
      <p v-else-if="!p" class="error">{{ error }}</p>

      <div v-else class="cuerpo">
        <div v-if="avisos.length" class="avisos">
          <p v-for="(a, i) in avisos" :key="i">{{ a }}</p>
        </div>

        <section>
          <h3>General</h3>
          <label><span>Nombre</span><input v-model="p.nombre" /></label>
          <div class="rejilla">
            <label><span>Ancho</span><input type="number" v-model.number="p.formato.ancho" /></label>
            <label><span>Alto</span><input type="number" v-model.number="p.formato.alto" /></label>
            <label><span>FPS</span><input type="number" v-model.number="p.formato.fps" /></label>
          </div>
        </section>

        <section>
          <h3>Guion</h3>
          <label>
            <span>Tono y voz narrativa</span>
            <textarea v-model="p.guion.tono" rows="3"></textarea>
          </label>
          <label>
            <span>Instrucciones adicionales</span>
            <textarea v-model="p.guion.instrucciones_extra" rows="3"></textarea>
          </label>
          <div class="rejilla">
            <label><span>Escenas</span><input type="number" v-model.number="p.guion.escenas" /></label>
            <label><span>Duración (s)</span><input type="number" v-model.number="p.guion.duracion_seg" /></label>
          </div>
        </section>

        <section>
          <h3>Imágenes</h3>
          <label>
            <span>Proveedor</span>
            <select v-model="p.imagen.proveedor">
              <option value="cloudflare">Cloudflare — rápido, sin semilla fija</option>
              <option value="pollinations">Pollinations — lento, mantiene personajes</option>
            </select>
          </label>
          <label>
            <span>Estilo <em>— se añade a todos los prompts</em></span>
            <textarea v-model="p.imagen.estilo" rows="3"></textarea>
          </label>
          <label><span>Semilla</span><input type="number" v-model.number="p.imagen.semilla" /></label>
        </section>

        <section>
          <h3>Voz</h3>
          <label>
            <span>Proveedor</span>
            <select v-model="p.voz.proveedor">
              <option value="piper">Piper — local, gratis</option>
              <option value="elevenlabs">ElevenLabs — mejor calidad, de pago</option>
            </select>
          </label>
          <label v-if="p.voz.proveedor === 'piper'">
            <span>Voz</span>
            <select v-model="p.voz.modelo">
              <option v-for="v in recursos.voces" :key="v.valor" :value="v.valor">{{ v.etiqueta }}</option>
            </select>
          </label>
          <label v-else>
            <span>Voice ID de ElevenLabs</span>
            <input v-model="p.voz.modelo" placeholder="lo copias de la web, en Voices" />
          </label>
          <div class="rejilla">
            <label><span>Velocidad</span><input type="number" step="0.02" v-model.number="p.voz.velocidad" /></label>
            <label><span>Expresividad</span><input type="number" step="0.05" v-model.number="p.voz.expresividad" /></label>
            <label><span>Presencia (dB)</span><input type="number" step="0.5" v-model.number="p.voz.presencia" /></label>
          </div>
          <label class="casilla">
            <input type="checkbox" v-model="p.voz.procesar" />
            <span>Procesar el audio — ecualiza, comprime y normaliza. Quita lo robótico</span>
          </label>
          <label v-if="p.voz.proveedor === 'elevenlabs'">
            <span>Límite de caracteres al mes <em>— al agotarse pasa a Piper</em></span>
            <input type="number" v-model.number="p.voz.limite_caracteres" />
          </label>
        </section>

        <section>
          <h3>Personaje</h3>
          <label>
            <span>Imagen o carpeta de expresiones</span>
            <select v-model="p.personaje.imagen">
              <option v-for="c in recursos.personajes" :key="c.valor" :value="c.valor">
                {{ c.etiqueta }}<template v-if="c.nota"> — {{ c.nota }}</template>
              </option>
            </select>
          </label>
          <template v-if="p.personaje.imagen">
            <div class="rejilla">
              <label>
                <span>Forma</span>
                <select v-model="p.personaje.forma">
                  <option value="circulo">Círculo</option>
                  <option value="recorte">Recorte (PNG con transparencia)</option>
                  <option value="croma">Croma</option>
                </select>
              </label>
              <label>
                <span>Posición</span>
                <select v-model="p.personaje.posicion">
                  <option value="abajo-derecha">Abajo derecha</option>
                  <option value="abajo-izquierda">Abajo izquierda</option>
                  <option value="abajo-centro">Abajo centro</option>
                </select>
              </label>
              <label>
                <span>Animación</span>
                <select v-model="p.personaje.animacion">
                  <option value="hablar">Al hablar</option>
                  <option value="respirar">Respirar</option>
                  <option value="ninguna">Quieto</option>
                </select>
              </label>
            </div>
            <div class="rejilla">
              <label><span>Tamaño (% del alto)</span><input type="number" v-model.number="p.personaje.alto_pct" /></label>
              <label><span>Margen (px)</span><input type="number" v-model.number="p.personaje.margen" /></label>
            </div>
          </template>
        </section>

        <section>
          <h3>Subtítulos</h3>
          <label class="casilla">
            <input type="checkbox" v-model="p.subtitulos.activos" /><span>Activos</span>
          </label>
          <template v-if="p.subtitulos.activos">
            <label>
              <span>Animación</span>
              <select v-model="p.subtitulos.animacion">
                <option value="pop">Pop — la línea entra con rebote</option>
                <option value="karaoke">Karaoke — resalta la palabra que se dice</option>
                <option value="palabra">Palabra — una a la vez, grande</option>
                <option value="ninguna">Estáticos</option>
              </select>
            </label>
            <div class="rejilla">
              <label><span>Tamaño (px)</span><input type="number" v-model.number="p.subtitulos.tam_px" /></label>
              <label><span>Altura (px)</span><input type="number" v-model.number="p.subtitulos.margen_v" /></label>
              <label><span>Palabras/línea</span><input type="number" v-model.number="p.subtitulos.palabras_por_linea" /></label>
            </div>
          </template>
        </section>

        <section>
          <h3>Montaje y audio</h3>
          <div class="rejilla">
            <label><span>Zoom</span><input type="number" step="0.02" v-model.number="p.video.zoom" /></label>
            <label><span>Mín. s/imagen</span><input type="number" step="0.2" v-model.number="p.video.min_seg_por_imagen" /></label>
            <label><span>Máx. s/imagen</span><input type="number" step="0.5" v-model.number="p.video.max_seg_por_imagen" /></label>
          </div>
          <label>
            <span>Música</span>
            <select v-model="p.video.musica">
              <option value="">(sin música)</option>
              <option v-for="m in recursos.musica" :key="m.valor" :value="m.valor">{{ m.etiqueta }}</option>
            </select>
          </label>
          <label>
            <span>Efecto en los cortes</span>
            <select v-model="p.video.efecto_transicion">
              <option value="">(sin efecto)</option>
              <option v-for="e in recursos.efectos" :key="e.valor" :value="e.valor">{{ e.etiqueta }}</option>
            </select>
          </label>
          <div class="rejilla">
            <label><span>Volumen música</span><input type="number" step="0.01" v-model.number="p.video.volumen_musica" /></label>
            <label><span>Volumen efectos</span><input type="number" step="0.02" v-model.number="p.video.volumen_efectos" /></label>
            <label>
              <span>Efectos en</span>
              <select v-model="p.video.efectos_en">
                <option value="escena">Cambio de escena</option>
                <option value="plano">Cada imagen</option>
                <option value="ninguno">Ninguno</option>
              </select>
            </label>
          </div>
        </section>
      </div>

      <footer v-if="p">
        <button class="principal" :disabled="guardando" @click="guardar">
          {{ guardando ? 'Guardando…' : 'Guardar' }}
        </button>
        <span v-if="guardado" class="ok pequeno">guardado</span>
        <span v-if="error" class="error pequeno">{{ error }}</span>
      </footer>
    </div>
  </div>
</template>

<style scoped>
.fondo {
  position: fixed; inset: 0; background: rgba(0, 0, 0, .45);
  display: flex; justify-content: center; align-items: flex-start;
  padding: 24px 16px; overflow-y: auto; z-index: 10;
}
.panel-editor {
  background: var(--panel); border: 1px solid var(--linea); border-radius: 12px;
  width: 100%; max-width: 660px; display: flex; flex-direction: column;
  max-height: calc(100vh - 48px);
}
header {
  display: flex; justify-content: space-between; align-items: center;
  padding: 16px 20px; border-bottom: 1px solid var(--linea);
}
h2 { font-size: 17px; margin: 0; }
.cuerpo { overflow-y: auto; padding: 4px 20px 20px; }

section { padding: 18px 0; border-bottom: 1px solid var(--linea); }
section:last-child { border-bottom: none; }
h3 {
  font-size: 12px; text-transform: uppercase; letter-spacing: .07em;
  color: var(--suave); margin: 0 0 12px; font-weight: 600;
}

label { display: block; margin-bottom: 12px; }
label > span { display: block; font-size: 13px; color: var(--suave); margin-bottom: 5px; }
label em { font-style: normal; opacity: .75; }
.rejilla { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; }
.casilla { display: flex; align-items: center; gap: 9px; }
.casilla input { width: auto; }
.casilla span { margin: 0; font-size: 13.5px; color: var(--tinta); }

/* Los avisos van arriba y no al guardar: es antes de gastar doce minutos
   cuando sirven de algo. */
.avisos {
  background: color-mix(in srgb, var(--aviso, #b4690e) 10%, transparent);
  border-left: 3px solid var(--aviso, #b4690e);
  border-radius: 0 8px 8px 0; padding: 12px 14px; margin: 16px 0 4px;
}
.avisos p { margin: 0 0 8px; font-size: 13.5px; line-height: 1.5; }
.avisos p:last-child { margin-bottom: 0; }

footer {
  display: flex; align-items: center; gap: 12px;
  padding: 14px 20px; border-top: 1px solid var(--linea);
}
.ok { color: var(--ok); }
.error { color: var(--error); }
</style>
