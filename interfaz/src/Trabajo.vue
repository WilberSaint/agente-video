<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

const props = defineProps({ t: { type: Object, required: true } })
defineEmits(['cancelar', 'olvidar', 'reintentar'])

const abierto = ref(false)
const verRegistro = ref(false)
const copiado = ref('')

// El portapapeles solo existe en contextos seguros; 127.0.0.1 cuenta como tal,
// pero si algún día se sirve por red plana hay que dejar el texto seleccionable
// en vez de fallar en silencio.
async function copiar(cual, texto) {
  try {
    await navigator.clipboard.writeText(texto)
    copiado.value = cual
    setTimeout(() => (copiado.value = ''), 1800)
  } catch (e) {
    copiado.value = ''
  }
}

const etiquetas = {
  en_cola: 'en cola',
  corriendo: 'generando',
  terminado: 'listo',
  fallido: 'falló',
  cancelado: 'cancelado',
}

const porcentaje = computed(() => Math.round((props.t.progreso || 0) * 100))
const activo = computed(() => props.t.estado === 'corriendo')

function comoTiempo(s) {
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

const duracion = computed(() => {
  if (!props.t.inicio || !props.t.fin) return ''
  return comoTiempo(Math.round((new Date(props.t.fin) - new Date(props.t.inicio)) / 1000))
})

// Un reloj propio: el progreso solo llega cuando cambia de etapa, y sin esto
// el tiempo transcurrido se quedaría congelado minutos entre avisos.
const ahora = ref(Date.now())
let reloj = null
onMounted(() => { reloj = setInterval(() => (ahora.value = Date.now()), 1000) })
onUnmounted(() => clearInterval(reloj))

const transcurrido = computed(() => {
  if (!props.t.inicio) return 0
  return Math.round((ahora.value - new Date(props.t.inicio)) / 1000)
})

// Estimación por regla de tres sobre lo ya hecho. Es aproximada —las etapas no
// duran lo mismo— pero en un proceso de doce minutos saber si quedan dos o diez
// cambia si te quedas mirando o te vas a hacer otra cosa.
const restante = computed(() => {
  const p = props.t.progreso || 0
  if (!activo.value || p < 0.04 || transcurrido.value < 8) return ''
  const total = transcurrido.value / p
  const falta = Math.max(0, Math.round(total - transcurrido.value))
  return `~${comoTiempo(falta)} restantes`
})
</script>

<template>
  <article class="tarjeta trabajo" :class="t.estado">
    <div class="cabecera">
      <div class="identidad">
        <span class="chip" :class="t.estado">{{ etiquetas[t.estado] || t.estado }}</span>
        <h3>{{ t.titulo || t.tema }}</h3>
        <p v-if="t.titulo" class="silencio pequeno tema">{{ t.tema }}</p>
      </div>
      <div class="botones">
        <span v-if="duracion" class="silencio pequeno">{{ duracion }}</span>
        <button v-if="activo || t.estado === 'en_cola'" class="tenue" @click="$emit('cancelar', t.id)">
          cancelar
        </button>
        <template v-else>
          <!-- Reintentar reencola el mismo tema. No empieza de cero: los
               checkpoints hacen que retome donde se quedó. -->
          <button v-if="t.estado === 'fallido'" class="tenue rehacer" @click="$emit('reintentar', t)">
            reintentar
          </button>
          <button class="tenue" @click="$emit('olvidar', t.id)">quitar</button>
        </template>
      </div>
    </div>

    <div v-if="activo" class="avance">
      <div class="barra"><div class="relleno" :style="{ width: porcentaje + '%' }"></div></div>
      <p class="pequeno silencio linea-avance">
        <strong>{{ t.etapa }}</strong>
        <span v-if="t.detalle"> · {{ t.detalle }}</span>
        <span class="por-ciento">{{ porcentaje }}%</span>
      </p>
      <p class="pequeno silencio tiempos">
        <span>{{ comoTiempo(transcurrido) }} en curso</span>
        <span v-if="restante">· {{ restante }}</span>
      </p>
    </div>

    <p v-if="t.estado === 'fallido'" class="fallo mono">{{ t.error }}</p>

    <div v-if="t.estado === 'terminado' && t.video" class="resultado">
      <button v-if="!abierto" @click="abierto = true">Ver video</button>
      <video v-else :src="`/media/${t.id}/video`" controls playsinline></video>
      <div v-if="t.publicacion" class="publicacion">
        <div class="campo">
          <span class="etiqueta">Título</span>
          <p class="valor">{{ t.publicacion.titulo }}</p>
          <button class="tenue pequeno" @click="copiar('titulo', t.publicacion.titulo)">
            {{ copiado === 'titulo' ? 'copiado' : 'copiar' }}
          </button>
        </div>
        <div class="campo">
          <span class="etiqueta">Descripción</span>
          <p class="valor">{{ t.publicacion.descripcion }}</p>
          <button class="tenue pequeno" @click="copiar('desc', t.publicacion.descripcion)">
            {{ copiado === 'desc' ? 'copiado' : 'copiar' }}
          </button>
        </div>
      </div>
      <a :href="`/media/${t.id}/textos`" target="_blank" class="enlace pequeno">
        abrir como archivo
      </a>
    </div>

    <div v-if="t.registro && t.registro.length" class="registro">
      <button class="tenue pequeno" @click="verRegistro = !verRegistro">
        {{ verRegistro ? 'ocultar' : 'ver' }} detalle
      </button>
      <pre v-if="verRegistro" class="mono">{{ t.registro.join('\n') }}</pre>
    </div>
  </article>
</template>

<style scoped>
.trabajo { margin-bottom: 10px; padding: 15px 17px; }
.trabajo.corriendo { border-color: color-mix(in srgb, var(--acento) 45%, var(--linea)); }
.trabajo.fallido { border-color: color-mix(in srgb, var(--error) 40%, var(--linea)); }

.cabecera { display: flex; justify-content: space-between; align-items: start; gap: 14px; }
.identidad { min-width: 0; }
h3 { margin: 6px 0 0; font-size: 16px; font-weight: 600; line-height: 1.35; }
.tema { margin: 3px 0 0; }
.botones { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }

.chip {
  display: inline-block; font-size: 11px; text-transform: uppercase;
  letter-spacing: 0.06em; padding: 2px 8px; border-radius: 999px;
  border: 1px solid var(--linea); color: var(--suave);
}
.chip.corriendo { color: var(--acento); border-color: color-mix(in srgb, var(--acento) 40%, transparent); }
.chip.terminado { color: var(--ok); border-color: color-mix(in srgb, var(--ok) 40%, transparent); }
.chip.fallido { color: var(--error); border-color: color-mix(in srgb, var(--error) 40%, transparent); }

.avance { margin-top: 13px; }
.barra { height: 6px; background: var(--linea); border-radius: 999px; overflow: hidden; }
.relleno {
  height: 100%; background: var(--acento); border-radius: 999px;
  /* La transición evita que la barra salte: el progreso llega a golpes,
     una vez por imagen generada. */
  transition: width .45s ease;
}
.linea-avance { display: flex; gap: 6px; margin: 7px 0 0; }
.por-ciento { margin-left: auto; font-variant-numeric: tabular-nums; }
.tiempos { display: flex; gap: 6px; margin: 4px 0 0; font-variant-numeric: tabular-nums; }

.fallo {
  margin: 12px 0 0; padding: 10px 12px; white-space: pre-wrap;
  background: color-mix(in srgb, var(--error) 8%, transparent);
  border-radius: 8px; color: var(--error);
}

.resultado { margin-top: 13px; display: flex; flex-direction: column; gap: 9px; align-items: start; }
video { width: 100%; max-width: 300px; border-radius: 8px; background: #000; display: block; }
.enlace { color: var(--acento); }

.publicacion { width: 100%; display: flex; flex-direction: column; gap: 10px; }
.campo {
  border: 1px solid var(--linea); border-radius: 8px; padding: 9px 11px;
  display: flex; flex-direction: column; align-items: start; gap: 5px;
}
.campo .etiqueta {
  font-size: 11px; text-transform: uppercase; letter-spacing: .06em; color: var(--suave);
}
/* pre-wrap porque la descripción trae el salto de línea antes de los hashtags,
   y esa separación es justo lo que se pega. */
.campo .valor { margin: 0; font-size: 14px; white-space: pre-wrap; }
.rehacer:hover { color: var(--acento); }

.registro { margin-top: 10px; }
.registro pre {
  margin: 8px 0 0; padding: 11px 13px; max-height: 260px; overflow: auto;
  background: color-mix(in srgb, var(--tinta) 5%, transparent);
  border-radius: 8px; white-space: pre-wrap; line-height: 1.5;
}
</style>
