<script setup>
import { ref, computed, onMounted, watch } from 'vue'

const props = defineProps({ perfiles: { type: Array, default: () => [] } })

const temas = ref([])
const reglas = ref([])
const perfil = ref('')
const nuevos = ref('')
const aviso = ref('')
const error = ref('')

const pendientes = computed(() => temas.value.filter((t) => t.estado === 'pendiente'))
const otros = computed(() => temas.value.filter((t) => t.estado !== 'pendiente'))

// Con la producción diaria configurada se puede decir cuánto durará el banco.
// Es el dato que importa: "quedan 12" no dice nada, "da para cuatro días" sí.
const diasDeBanco = computed(() => {
  const porDia = reglas.value
    .filter((r) => r.activa && r.perfil === perfil.value)
    .reduce((n, r) => n + (r.cantidad || 0), 0)
  if (!porDia) return ''
  return `da para ${Math.floor(pendientes.value.length / porDia)} día(s) a ${porDia}/día`
})

async function cargar() {
  try {
    const [rt, rr] = await Promise.all([
      fetch(`/api/temas?perfil=${perfil.value}`),
      fetch('/api/horario'),
    ])
    temas.value = (await rt.json()) || []
    reglas.value = (await rr.json()) || []
  } catch (e) {
    error.value = 'no se pudo cargar la producción'
  }
}

async function agregar() {
  if (!nuevos.value.trim()) return
  error.value = ''
  aviso.value = ''
  try {
    const r = await fetch('/api/temas', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ perfil: perfil.value, texto: nuevos.value }),
    })
    const d = await r.json()
    if (!r.ok) throw new Error(d.error || 'no se pudo guardar')
    temas.value = d.temas || []
    nuevos.value = ''
    aviso.value = d.repetidos
      ? `${d.nuevos} añadidos, ${d.repetidos} ya estaban`
      : `${d.nuevos} añadidos`
    setTimeout(() => (aviso.value = ''), 3500)
  } catch (e) {
    error.value = e.message
  }
}

async function cambiar(t, estado) {
  await fetch(`/api/temas/${t.id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ estado }),
  })
  cargar()
}

async function olvidar(t) {
  await fetch(`/api/temas/${t.id}`, { method: 'DELETE' })
  temas.value = temas.value.filter((x) => x.id !== t.id)
}

function reglaNueva() {
  return {
    id: '', activa: true, perfil: perfil.value || (props.perfiles[0] || {}).id,
    hora: '03:00', dias: [], cantidad: 3, proponer_si_faltan: false,
  }
}
const editando = ref(null)

async function guardarRegla() {
  error.value = ''
  try {
    const r = await fetch('/api/horario', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(editando.value),
    })
    const d = await r.json()
    if (!r.ok) throw new Error(d.error || 'no se pudo guardar')
    editando.value = null
    cargar()
  } catch (e) {
    error.value = e.message
  }
}

async function olvidarRegla(r) {
  await fetch(`/api/horario/${r.id}`, { method: 'DELETE' })
  cargar()
}

const nombresDia = ['do', 'lu', 'ma', 'mi', 'ju', 'vi', 'sá']
function alternarDia(d) {
  const i = editando.value.dias.indexOf(d)
  if (i >= 0) editando.value.dias.splice(i, 1)
  else editando.value.dias.push(d)
}
function resumenDias(r) {
  if (!r.dias || !r.dias.length) return 'todos los días'
  return [...r.dias].sort().map((d) => nombresDia[d]).join(' ')
}
function nombrePerfil(id) {
  return (props.perfiles.find((p) => p.id === id) || {}).nombre || id
}

watch(perfil, cargar)
onMounted(() => {
  perfil.value = (props.perfiles[0] || {}).id || ''
  cargar()
})
</script>

<template>
  <div class="produccion">
    <section class="tarjeta">
      <div class="cabeza">
        <h3>Banco de temas</h3>
        <select v-model="perfil" class="selector">
          <option v-for="p in perfiles" :key="p.id" :value="p.id">{{ p.nombre }}</option>
        </select>
      </div>

      <p class="silencio pequeno estado-banco">
        <strong>{{ pendientes.length }}</strong> pendiente(s)
        <span v-if="diasDeBanco">· {{ diasDeBanco }}</span>
      </p>

      <textarea
        v-model="nuevos"
        rows="3"
        placeholder="una idea por línea&#10;el faro que nadie visita&#10;por qué olvidamos los sueños"
        @keydown.ctrl.enter="agregar"
      ></textarea>
      <div class="acciones">
        <button class="principal" :disabled="!nuevos.trim()" @click="agregar">Añadir al banco</button>
        <span v-if="aviso" class="ok pequeno">{{ aviso }}</span>
        <span v-if="error" class="error pequeno">{{ error }}</span>
      </div>

      <ul v-if="pendientes.length" class="lista">
        <li v-for="t in pendientes" :key="t.id">
          <span class="texto">{{ t.texto }}</span>
          <span v-if="t.origen === 'propuesto'" class="marca">propuesto</span>
          <button class="tenue pequeno" @click="cambiar(t, 'descartado')">descartar</button>
        </li>
      </ul>

      <details v-if="otros.length" class="usados">
        <summary class="silencio pequeno">{{ otros.length }} usado(s) o descartado(s)</summary>
        <ul class="lista">
          <li v-for="t in otros" :key="t.id" class="apagado">
            <span class="texto">{{ t.texto }}</span>
            <span class="marca">{{ t.estado }}</span>
            <button class="tenue pequeno" @click="cambiar(t, 'pendiente')">reutilizar</button>
            <button class="tenue pequeno" @click="olvidar(t)">quitar</button>
          </li>
        </ul>
      </details>
    </section>

    <section class="tarjeta">
      <div class="cabeza">
        <h3>Horario</h3>
        <button v-if="!editando" @click="editando = reglaNueva()">Nueva regla</button>
      </div>

      <p v-if="!reglas.length && !editando" class="silencio pequeno">
        Sin reglas. Con una regla activa, el agente genera solo a la hora que le digas,
        tomando los temas del banco.
      </p>

      <div v-for="r in reglas" :key="r.id" class="regla" :class="{ apagada: !r.activa }">
        <div class="linea">
          <strong>{{ r.hora }}</strong>
          <span>{{ r.cantidad }} video(s) de {{ nombrePerfil(r.perfil) }}</span>
          <span class="silencio">· {{ resumenDias(r) }}</span>
          <span v-if="!r.activa" class="marca">pausada</span>
        </div>
        <p v-if="r.ultimo_resumen" class="silencio pequeno ultimo">
          último disparo: {{ r.ultimo_resumen }}
        </p>
        <div class="acciones">
          <button class="tenue pequeno" @click="editando = { ...r, dias: [...(r.dias || [])] }">editar</button>
          <button class="tenue pequeno" @click="olvidarRegla(r)">quitar</button>
        </div>
      </div>

      <div v-if="editando" class="formulario">
        <div class="rejilla">
          <label>
            <span>Perfil</span>
            <select v-model="editando.perfil">
              <option v-for="p in perfiles" :key="p.id" :value="p.id">{{ p.nombre }}</option>
            </select>
          </label>
          <label><span>Hora</span><input type="time" v-model="editando.hora" /></label>
          <label><span>Videos</span><input type="number" min="1" v-model.number="editando.cantidad" /></label>
        </div>

        <span class="etiqueta">Días <em>— ninguno marcado = todos</em></span>
        <div class="dias">
          <button
            v-for="(n, d) in nombresDia" :key="d"
            :class="{ activo: editando.dias.includes(d) }"
            @click="alternarDia(d)"
          >{{ n }}</button>
        </div>

        <label class="casilla">
          <input type="checkbox" v-model="editando.proponer_si_faltan" />
          <span>Proponer temas si el banco no da — sin esto, un banco vacío es una noche perdida</span>
        </label>
        <label class="casilla">
          <input type="checkbox" v-model="editando.activa" /><span>Activa</span>
        </label>

        <div class="acciones">
          <button class="principal" @click="guardarRegla">Guardar regla</button>
          <button class="tenue" @click="editando = null">cancelar</button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.produccion { display: flex; flex-direction: column; gap: 12px; }
.cabeza { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 8px; }
h3 { font-size: 15px; margin: 0; }
.selector { width: auto; max-width: 260px; }
.estado-banco { margin: 0 0 12px; }

textarea { margin-bottom: 10px; }
.acciones { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.ok { color: var(--ok); }
.error { color: var(--error); }

.lista { list-style: none; margin: 14px 0 0; padding: 0; }
.lista li {
  display: flex; align-items: center; gap: 8px;
  padding: 7px 0; border-top: 1px solid var(--linea); font-size: 14px;
}
.lista .texto { flex: 1; min-width: 0; }
.apagado .texto { color: var(--suave); text-decoration: line-through; }
.marca {
  font-size: 11px; text-transform: uppercase; letter-spacing: .05em;
  color: var(--suave); border: 1px solid var(--linea); border-radius: 999px; padding: 1px 7px;
}
.usados { margin-top: 14px; }
.usados summary { cursor: pointer; }

.regla { border-top: 1px solid var(--linea); padding: 11px 0; }
.regla.apagada { opacity: .55; }
.linea { display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap; font-size: 14.5px; }
.ultimo { margin: 4px 0 0; }

.formulario { border-top: 1px solid var(--linea); margin-top: 12px; padding-top: 14px; }
.rejilla { display: grid; grid-template-columns: repeat(auto-fit, minmax(130px, 1fr)); gap: 12px; }
label { display: block; margin-bottom: 12px; }
label > span, .etiqueta { display: block; font-size: 13px; color: var(--suave); margin-bottom: 5px; }
label em { font-style: normal; opacity: .75; }
.casilla { display: flex; align-items: center; gap: 9px; }
.casilla input { width: auto; }
.casilla span { margin: 0; font-size: 13.5px; color: var(--tinta); }

.dias { display: flex; gap: 6px; margin-bottom: 14px; flex-wrap: wrap; }
.dias button { padding: 6px 11px; min-width: 42px; }
.dias button.activo { background: var(--acento); border-color: var(--acento); color: #fff; }
</style>
