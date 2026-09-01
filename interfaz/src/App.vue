<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import Trabajo from './Trabajo.vue'
import Editor from './Editor.vue'

const perfiles = ref([])
const trabajos = ref([])
const perfil = ref('')
const temas = ref('')
const enviando = ref(false)
const error = ref('')
const conectado = ref(false)

let fuente = null

// Un tema por línea. Escribir cinco y encolarlos de golpe es la diferencia
// entre lanzar un video y dejar la máquina trabajando toda la noche.
const listaTemas = computed(() =>
  temas.value.split('\n').map((t) => t.trim()).filter(Boolean)
)

const enCurso = computed(() => trabajos.value.filter((t) => t.estado === 'corriendo'))
const enCola = computed(() => trabajos.value.filter((t) => t.estado === 'en_cola'))
const listos = computed(() => trabajos.value.filter((t) => t.estado === 'terminado'))
const cerrados = computed(() =>
  trabajos.value.filter((t) => ['fallido', 'cancelado'].includes(t.estado))
)

// Cada video tarda unos doce minutos y se hacen en serie: con cinco en cola,
// saber que son dos horas cambia si te esperas o te vas.
const esperaTotal = computed(() => {
  const n = enCola.value.length + enCurso.value.length
  if (!n) return ''
  const min = Math.round(n * 12)
  if (min < 60) return `~${min} min en total`
  const h = Math.floor(min / 60)
  return `~${h}h ${min % 60}min en total`
})

function fusionar(t) {
  const i = trabajos.value.findIndex((x) => x.id === t.id)
  if (i >= 0) trabajos.value[i] = t
  else trabajos.value.unshift(t)
}

function conectar() {
  fuente = new EventSource('/api/eventos')
  fuente.addEventListener('lista', (e) => {
    trabajos.value = JSON.parse(e.data) || []
    conectado.value = true
  })
  fuente.addEventListener('estado', (e) => fusionar(JSON.parse(e.data)))
  fuente.onopen = () => (conectado.value = true)
  // EventSource reintenta solo; solo hay que reflejar que se cayó.
  fuente.onerror = () => (conectado.value = false)
}

async function cargarPerfiles() {
  try {
    const r = await fetch('/api/perfiles')
    perfiles.value = await r.json()
    if (!perfil.value && perfiles.value.length) perfil.value = perfiles.value[0].id
  } catch (e) {
    error.value = 'no se pudieron cargar los perfiles'
  }
}

async function encolar() {
  if (!listaTemas.value.length || enviando.value) return
  enviando.value = true
  error.value = ''
  try {
    const r = await fetch('/api/trabajos', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ perfil: perfil.value, temas: listaTemas.value }),
    })
    if (!r.ok) throw new Error((await r.json()).error || 'no se pudo encolar')
    temas.value = ''
  } catch (e) {
    error.value = e.message
  } finally {
    enviando.value = false
  }
}

async function cancelar(id) {
  await fetch(`/api/trabajos/${id}/cancelar`, { method: 'POST' })
}

async function olvidar(id) {
  await fetch(`/api/trabajos/${id}`, { method: 'DELETE' })
  trabajos.value = trabajos.value.filter((t) => t.id !== id)
}

// Reintentar es volver a encolar el mismo tema: los checkpoints hacen que
// retome donde se quedó en vez de empezar de cero.
async function reintentar(t) {
  await fetch('/api/trabajos', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ perfil: t.perfil, tema: t.tema }),
  })
}

const perfilActual = computed(() => perfiles.value.find((p) => p.id === perfil.value))
const editando = ref('')

onMounted(() => {
  cargarPerfiles()
  conectar()
})
onUnmounted(() => fuente && fuente.close())
</script>

<template>
  <div class="envoltura">
    <header>
      <h1>agente-video</h1>
      <span class="estado-conexion" :class="{ vivo: conectado }">
        {{ conectado ? 'en vivo' : 'reconectando…' }}
      </span>
    </header>

    <section class="tarjeta lanzador">
      <div class="fila-perfil">
        <label>
          <span class="etiqueta">Perfil</span>
          <select v-model="perfil">
            <option v-for="p in perfiles" :key="p.id" :value="p.id">{{ p.nombre }}</option>
          </select>
        </label>
        <button class="editar" :disabled="!perfil" @click="editando = perfil">Editar perfil</button>
        <p v-if="perfilActual" class="silencio pequeno resumen-perfil">
          {{ perfilActual.formato }} · {{ perfilActual.escenas }} escenas · ~{{ perfilActual.segundos }}s
        </p>
      </div>

      <label>
        <span class="etiqueta">
          Temas <span class="silencio">— uno por línea</span>
        </span>
        <textarea
          v-model="temas"
          rows="4"
          placeholder="el faro que nadie visita&#10;por qué olvidamos los sueños&#10;quién inventó el cero"
          @keydown.ctrl.enter="encolar"
        ></textarea>
      </label>

      <div class="acciones">
        <button class="principal" :disabled="!listaTemas.length || enviando" @click="encolar">
          {{ listaTemas.length > 1 ? `Encolar ${listaTemas.length} videos` : 'Generar video' }}
        </button>
        <span class="silencio pequeno atajo">Ctrl+Enter</span>
        <span v-if="listaTemas.length > 1" class="silencio pequeno">
          ~{{ Math.round(listaTemas.length * 12) }} min de generación
        </span>
        <span v-if="error" class="error pequeno">{{ error }}</span>
      </div>
    </section>

    <template v-if="enCurso.length">
      <h2>En curso</h2>
      <Trabajo v-for="t in enCurso" :key="t.id" :t="t"
               @cancelar="cancelar" @olvidar="olvidar" @reintentar="reintentar" />
    </template>

    <template v-if="enCola.length">
      <h2>
        En cola <span class="silencio">({{ enCola.length }})</span>
        <span v-if="esperaTotal" class="silencio normal">— {{ esperaTotal }}</span>
      </h2>
      <Trabajo v-for="t in enCola" :key="t.id" :t="t"
               @cancelar="cancelar" @olvidar="olvidar" @reintentar="reintentar" />
    </template>

    <!-- Siempre visible, incluso vacía: una sección que aparece y desaparece
         hace dudar de si el video se guardó en algún sitio. -->
    <h2>Listos <span v-if="listos.length" class="silencio">({{ listos.length }})</span></h2>
    <Trabajo v-for="t in listos" :key="t.id" :t="t"
             @cancelar="cancelar" @olvidar="olvidar" @reintentar="reintentar" />
    <p v-if="!listos.length" class="tarjeta silencio pequeno vacio-seccion">
      Aún no hay videos terminados. Los que generes aparecerán aquí y podrás verlos
      sin salir de esta página.
    </p>

    <template v-if="cerrados.length">
      <h2>Fallidos y cancelados</h2>
      <Trabajo v-for="t in cerrados" :key="t.id" :t="t"
               @cancelar="cancelar" @olvidar="olvidar" @reintentar="reintentar" />
    </template>
  <Editor v-if="editando" :perfil-id="editando"
            @cerrar="editando = ''" @guardado="cargarPerfiles" />

  </div>
</template>

<style scoped>
.envoltura { max-width: 780px; margin: 0 auto; padding: 32px 20px 80px; }

header { display: flex; align-items: baseline; gap: 14px; margin-bottom: 22px; }
h1 { font-size: 21px; margin: 0; letter-spacing: -0.02em; }

.estado-conexion {
  font-size: 12px; color: var(--suave);
  border: 1px solid var(--linea); border-radius: 999px; padding: 2px 10px;
}
.estado-conexion.vivo { color: var(--ok); border-color: color-mix(in srgb, var(--ok) 40%, transparent); }

.lanzador { display: flex; flex-direction: column; gap: 14px; }
.fila-perfil { display: flex; align-items: end; gap: 14px; flex-wrap: wrap; }
.fila-perfil label { flex: 1; min-width: 220px; }
.resumen-perfil { margin: 0 0 10px; }
.editar { margin-bottom: 10px; white-space: nowrap; }

label { display: block; }
.etiqueta { display: block; font-size: 13px; color: var(--suave); margin-bottom: 6px; }

.acciones { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.error { color: var(--error); }

h2 { font-size: 14px; text-transform: uppercase; letter-spacing: 0.07em;
     color: var(--suave); margin: 30px 0 10px; font-weight: 600; }
h2 .normal { text-transform: none; letter-spacing: 0; font-weight: 400; }

.vacio-seccion { text-align: center; padding: 22px 18px; }

/* En móvil la acción principal ocupa el ancho: es el objetivo de la pantalla
   y el pulgar no debería tener que buscarla. */
@media (max-width: 560px) {
  .acciones { gap: 8px; }
  .acciones .principal { width: 100%; }
  .atajo { display: none; }
}
</style>
