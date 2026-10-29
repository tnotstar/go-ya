# ya-client — smoke-test del gateway go-ya

Cliente de pruebas mínimo que verifica que el gateway [go-ya](../../README.md)
(o cualquier servidor compatible con la API de OpenAI) responde correctamente:
consulta `GET /models`, ejecuta un `POST /chat/completions` y reporta estados,
latencias y uso de tokens.

Asume por defecto que el gateway corre en la misma máquina
(`http://localhost:8080/v1`) y que el modelo se especifica con el prefijo de
proveedor del gateway (`provider:model`, p. ej. `openai:gpt-4o-mini`).

## Requisitos y preparación

Solo necesitas [`uv`](https://docs.astral.sh/uv/). El entorno se prepara con:

```bash
cd scripts/ya-client
uv sync          # crea .venv e instala httpx según uv.lock
```

## Uso

El mensaje del usuario se lee de **stdin**. La respuesta del asistente va a
**stdout** (solo el texto) y toda la metadata de la verificación a **stderr**.

```bash
# Check completo contra el gateway local (models + chat)
echo "Hola" | uv run ya-client.py --model openai:gpt-6-luna

# Respuesta en streaming (SSE), con uso de tokens del chunk final
echo "Explícame los closures" | uv run ya-client.py --stream

# Formato JSON forzado
echo "Dame una lista de 3 colores" | uv run ya-client.py \
    --response-format json_object --temperature 0

# Con JSON Schema (fichero local con el esquema)
echo "Datos de contacto" | uv run ya-client.py \
    --response-format json_schema --json-schema-file contacto.schema.json

# Contra un servidor OpenAI-compatible remoto (la API real de OpenAI)
echo "Hola" | uv run ya-client.py \
    --baseurl https://api.openai.com/v1 --model gpt-4o-mini \
    --envar OPENAI_API_KEY
```

> Nota: en Nushell, `echo "Hola" | uv run ya-client.py …` funciona igual que en
> bash (stdin se hereda del pipe).

## Flags principales

| Flag | Por defecto | Descripción |
|---|---|---|
| `--baseurl URL` | `http://localhost:8080/v1` | URL base, incluido `/v1` si procede |
| `--model MODELO` | `openai:gpt-4o-mini` | modelo; el gateway enruta por prefijo `provider:model` |
| `--envar VAR` | `OPENAI_API_KEY` | variable de entorno con la API key; vacía ⇒ sin cabecera `Authorization` (gateway sin auth) |
| `--stream` | off | respuesta en streaming SSE |
| `--system TEXTO` | — | prompt de sistema |
| `--response-format` | `text` | `text` · `json_object` · `json_schema` (+ `--json-schema-file`) |
| `--param N=V` | — | parámetro extra del cuerpo JSON (repetible; valor como JSON o cadena) |
| `--header N:V` | — | cabecera HTTP extra (repetible) |
| `--skip-models` | off | no consultar `GET /models` antes del chat |
| `--timeout SEG` | 60 | timeout de conexión y lectura |
| `-v / --verbose` | off | volcar petición y respuesta cruda por stderr |

## Códigos de salida

| Código | Significado |
|---|---|
| `0` | verificación OK (models consultado sin fallo y respuesta de texto recibida) |
| `1` | fallo de verificación (conexión, HTTP de error, respuesta sin texto) |
| `2` | error de uso/configuración (stdin vacío, flags malformados, falta httpx) |

## Estructura del proyecto

Proyecto [`uv`](https://docs.astral.sh/uv/) de fichero único: `ya-client.py` es
el único fuente; `httpx` es la única dependencia y queda fijada en `uv.lock`.
