#!/usr/bin/env python3
"""ya-client.py — comprueba que el gateway go-ya (o cualquier servidor compatible
con la API de OpenAI) responde.

Por defecto asume que el gateway corre en la misma máquina
(http://localhost:8080/v1) y enruta por prefijo de proveedor en el nombre
del modelo (p. ej. openai:gpt-4o-mini).

Lee el mensaje del usuario desde stdin, consulta opcionalmente GET /models y
ejecuta un POST a {baseurl}/chat/completions usando httpx, con los parámetros
de llamada que se indiquen por línea de comandos.

Salidas:
  - stdout: SOLO el texto de la respuesta del asistente
  - stderr: TODA la metadata de la verificación (endpoints, estados, tiempos,
    tokens de uso, avisos y errores)
  - exit code: 0 OK · 1 fallo de verificación · 2 error de uso/configuración
"""

import argparse
import json
import os
import sys
import time
from pathlib import Path

try:
    import httpx
except ImportError:  # pragma: no cover
    sys.stderr.write("[error] falta la dependencia 'httpx'. Instálala con: uv sync\n")
    sys.exit(2)


def log(msg: str) -> None:
    print(msg, file=sys.stderr, flush=True)


def die1(msg: str, detail: str = "") -> "NoReturn":
    print(f"[error] {msg}", file=sys.stderr, flush=True)
    if detail:
        print(detail[:2000], file=sys.stderr, flush=True)
    sys.exit(1)


def hint_connection() -> str:
    return ("pista: revisa que el gateway esté levantado en localhost:8080 (o el "
            "servidor indicado con --baseurl), que la URL incluya el prefijo /v1 "
            "cuando corresponda, el puerto y el firewall")


def parse_extra_params(pairs: list[str], parser: argparse.ArgumentParser) -> dict:
    """Convierte '--param nombre=valor' en un dict; el valor se interpreta como
    JSON y, si no lo es, se usa como cadena."""
    out: dict = {}
    for pair in pairs:
        if "=" not in pair:
            parser.error(f"--param debe tener forma nombre=valor (recibido: {pair!r})")
        key, raw = pair.split("=", 1)
        key = key.strip()
        if not key:
            parser.error(f"--param: nombre vacío en {pair!r}")
        try:
            out[key] = json.loads(raw)
        except json.JSONDecodeError:
            out[key] = raw
    return out


def build_headers(args: argparse.Namespace, parser: argparse.ArgumentParser) -> dict[str, str]:
    headers = {"User-Agent": "ya-client/0.1"}
    var = args.envar.lstrip("$")
    key = os.environ.get(var, "").strip()
    if key:
        headers["Authorization"] = f"Bearer {key}"
    else:
        log(f"[aviso] la variable de entorno {var!r} no está definida o está vacía; "
            "se intentará sin cabecera Authorization (válido cuando el gateway "
            "no exige tokens de autenticación)")
    for pair in args.header:
        if ":" not in pair:
            parser.error(f"--header debe tener forma 'Nombre: valor' (recibido: {pair!r})")
        name, _, value = pair.partition(":")
        headers[name.strip()] = value.strip()
    return headers


def build_response_format(args: argparse.Namespace) -> dict | None:
    if args.response_format == "json_object":
        return {"type": "json_object"}
    if args.response_format == "json_schema":
        if not args.schema_file:
            die1("--response-format json_schema requiere --json-schema-file")
        try:
            schema = json.loads(Path(args.schema_file).read_text(encoding="utf-8"))
        except FileNotFoundError:
            die1(f"--json-schema-file: no existe el fichero {args.schema_file!r}")
        except json.JSONDecodeError as exc:
            die1(f"--json-schema-file: JSON inválido ({exc})")
        name = args.schema_name or Path(args.schema_file).stem or "response"
        return {"type": "json_schema", "json_schema": {"name": name, "schema": schema}}
    return None  # "text": es el valor por defecto del API, no se envía


def build_payload(args: argparse.Namespace, prompt: str, extra: dict) -> dict:
    messages = []
    if args.system:
        messages.append({"role": "system", "content": args.system})
    messages.append({"role": "user", "content": prompt})

    payload: dict = {"model": args.model, "messages": messages}
    scalars = {
        "temperature": args.temperature,
        "top_p": args.top_p,
        "max_tokens": args.max_tokens,
        "n": args.n,
        "presence_penalty": args.presence_penalty,
        "frequency_penalty": args.frequency_penalty,
        "seed": args.seed,
    }
    for key, value in scalars.items():
        if value is not None:
            payload[key] = value
    if args.stop:
        payload["stop"] = list(args.stop)
    rf = build_response_format(args)
    if rf is not None:
        payload["response_format"] = rf
    if args.stream:
        payload["stream"] = True
        if not args.no_stream_options:
            payload["stream_options"] = {"include_usage": True}
    payload.update(extra)
    return payload


def check_models(client: httpx.Client, baseurl: str) -> None:
    url = f"{baseurl.rstrip('/')}/models"
    log(f"check: GET {url}")
    try:
        resp = client.get(url)
    except httpx.HTTPError as exc:
        die1(f"fallo de conexión consultando modelos ({type(exc).__name__}: {exc})",
             hint_connection())
    if resp.status_code == 200:
        try:
            data = resp.json()
        except ValueError:
            log("[aviso] GET /models respondió 200 pero el cuerpo no es JSON válido")
            return
        models = data.get("data") or []
        ids = ", ".join(str(m.get("id")) for m in models[:10])
        log(f"check: 200 OK — {len(models)} modelos disponibles"
            + (f" (primeros: {ids})" if models else ""))
    elif resp.status_code in (401, 403):
        die1(f"GET /models respondió {resp.status_code} — API key ausente o inválida",
             resp.text)
    else:
        log(f"[aviso] GET /models respondió HTTP {resp.status_code}; "
            "el servidor puede no soportar la lista de modelos")


def extract_content(choice: dict) -> tuple[str | None, bool]:
    """Devuelve (texto, es_de_herramientas) del message de una elección."""
    msg = choice.get("message") or {}
    content = msg.get("content")
    if isinstance(content, list):  # contenido en bloques (modelos tipo o-series)
        text = "".join(p.get("text", "") for p in content if isinstance(p, dict))
        return text, False
    if content is None:
        return None, bool(msg.get("tool_calls"))
    return str(content), False


def chat_non_stream(client: httpx.Client, url: str, payload: dict,
                    headers: dict, verbose: bool) -> None:
    t0 = time.monotonic()
    try:
        resp = client.post(url, json=payload, headers=headers)
    except httpx.TimeoutException as exc:
        die1(f"timeout tras {time.monotonic() - t0:.1f} s en la llamada de chat ({exc})",
             hint_connection())
    except httpx.HTTPError as exc:
        die1(f"fallo de conexión en la llamada de chat ({type(exc).__name__}: {exc})",
             hint_connection())
    dt = time.monotonic() - t0
    log(f"check: POST {url} → HTTP {resp.status_code} en {dt:.2f} s")
    if verbose:
        log(f"resp: {resp.text[:4000]}")
    if resp.status_code != 200:
        detail = resp.text[:2000] if resp.text else "(cuerpo vacío)"
        die1(f"el servidor devolvió HTTP {resp.status_code}", detail)
    try:
        data = resp.json()
    except ValueError:
        die1("el servidor devolvió HTTP 200 con un cuerpo que no es JSON válido")

    choices = data.get("choices") or []
    if not choices:
        die1("respuesta sin 'choices' (estructura no compatible con Chat Completions)",
             json.dumps(data, ensure_ascii=False)[:2000])
    content, is_tool = extract_content(choices[0])
    if content is None:
        if is_tool:
            die1("el servidor respondió con tool_calls en lugar de texto; "
                 "este check valida respuestas de texto — añade --system para guiar al modelo")
        die1("elecciones presentes pero message.content vacío o ausente",
             json.dumps(choices[0], ensure_ascii=False)[:2000])
    if content == "":
        log("[aviso] el servidor devolvió una cadena de texto vacía")

    sys.stdout.write(content + "\n")
    sys.stdout.flush()

    usage = data.get("usage") or {}
    log(f"check: id={data.get('id', '(sin id)')} "
        f"model={data.get('model', '(sin eco de modelo)')} "
        f"choices={len(choices)} "
        f"finish={choices[0].get('finish_reason', '?')}")
    if usage:
        log(f"check: uso de tokens: prompt={usage.get('prompt_tokens', '?')} "
            f"completion={usage.get('completion_tokens', '?')} "
            f"total={usage.get('total_tokens', '?')}")
    else:
        log("[aviso] la respuesta no incluye 'usage' (algunos servidores no lo devuelven)")
    log(f"check: OK — respuesta de {len(content)} caracteres en {dt:.2f} s")


def chat_stream(client: httpx.Client, url: str, payload: dict,
                headers: dict, verbose: bool) -> None:
    t0 = time.monotonic()
    first_ms: float | None = None
    parts: list[str] = []
    finish = "?"
    usage: dict = {}
    chunks = 0
    err_lines: list[str] = []
    try:
        with client.stream("POST", url, json=payload, headers=headers) as resp:
            log(f"check: POST {url} → HTTP {resp.status_code} (streaming)")
            if resp.status_code != 200:
                body = resp.read().decode("utf-8", "replace")
                die1(f"el servidor devolvió HTTP {resp.status_code}", body[:2000] or "(cuerpo vacío)")
            for line in resp.iter_lines():
                if line == "" or line.startswith(":"):
                    continue  # keep-alive / comentario
                if verbose:
                    err_lines.append(line)
                if not line.startswith("data:"):
                    continue
                payload_s = line[5:].strip()
                if payload_s == "[DONE]":
                    break
                try:
                    chunk = json.loads(payload_s)
                except json.JSONDecodeError:
                    log(f"[aviso] fragmento SSE no parseable como JSON: {payload_s[:120]}")
                    continue
                chunks += 1
                if first_ms is None:
                    first_ms = (time.monotonic() - t0) * 1000
                if isinstance(chunk.get("usage"), dict):
                    usage = chunk["usage"]  # chunk final con stream_options.include_usage
                for ch in chunk.get("choices") or []:
                    delta = ch.get("delta") or {}
                    piece = delta.get("content")
                    if isinstance(piece, list):
                        piece = "".join(p.get("text", "") for p in piece if isinstance(p, dict))
                    if piece:
                        parts.append(piece)
                        sys.stdout.write(piece)
                        sys.stdout.flush()
                    if ch.get("finish_reason"):
                        finish = ch["finish_reason"]
    except httpx.TimeoutException as exc:
        if not parts:
            die1(f"timeout en streaming sin datos recibidos ({exc})", hint_connection())
        log(f"[aviso] timeout durante el streaming tras {len(parts)} fragmentos")
    except httpx.HTTPError as exc:
        if not parts:
            die1(f"fallo de conexión en streaming ({type(exc).__name__}: {exc})",
                 hint_connection())
        log(f"[aviso] el streaming se interrumpió: {type(exc).__name__}: {exc}")

    dt = time.monotonic() - t0
    text = "".join(parts)
    if text:
        sys.stdout.write("\n")
    sys.stdout.flush()
    if verbose:
        log(f"resp: {chunks} líneas SSE recibidas:\n" + "\n".join(err_lines[:200]))

    if not text:
        die1("el streaming no produjo ningún texto")

    log(f"check: {chunks} fragmentos SSE · primer byte en "
        f"{first_ms:.0f} ms · total {dt:.2f} s · finish={finish}")
    if usage:
        log(f"check: uso de tokens: prompt={usage.get('prompt_tokens', '?')} "
            f"completion={usage.get('completion_tokens', '?')} "
            f"total={usage.get('total_tokens', '?')}")
    else:
        log("[aviso] el stream no incluyó uso de tokens "
            "(¿servidor que ignora stream_options.include_usage?)")
    log(f"check: OK — respuesta de {len(text)} caracteres en streaming")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="ya-client.py",
        description="Verifica el gateway go-ya (o cualquier servidor compatible con la "
                    "API de OpenAI). El prompt se lee de stdin; la respuesta del asistente "
                    "va a stdout y toda la metadata de la verificación a stderr.",
        epilog="Ejemplos:\n"
               '  echo "Hola" | uv run ya-client.py --model openai:gpt-6-luna\n'
               '  echo "Hola" | uv run ya-client.py --stream\n'
               '  echo "Dame un JSON" | uv run ya-client.py --response-format json_object '
               '--temperature 0\n'
               '  echo "Hola" | uv run ya-client.py --baseurl https://api.openai.com/v1 '
               '--model gpt-4o-mini --envar OPENAI_API_KEY\n'
               "Códigos de salida: 0 verificación OK · 1 fallo de verificación "
               "· 2 error de uso\n",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--envar", default="OPENAI_API_KEY",
                        help="variable de entorno con la API key (por defecto: OPENAI_API_KEY; "
                             "si está vacía no se envía Authorization, válido para el gateway "
                             "sin autenticación)")
    parser.add_argument("--baseurl", default="http://localhost:8080/v1",
                        help="URL base del gateway go-ya o de cualquier API compatible con "
                             "OpenAI, incluyendo /v1 si procede "
                             "(por defecto: http://localhost:8080/v1, el gateway en "
                             "la misma máquina)")
    parser.add_argument("--model", default="openai:gpt-4o-mini",
                        help="modelo de la llamada de prueba; el gateway enruta por prefijo "
                             "provider:model (por defecto: openai:gpt-4o-mini)")
    parser.add_argument("--system", help="prompt de sistema opcional")
    parser.add_argument("--temperature", type=float, help="parámetro temperature")
    parser.add_argument("--top-p", type=float, help="parámetro top_p")
    parser.add_argument("--max-tokens", type=int,
                        help="parámetro max_tokens (para max_completion_tokens usa "
                             "--param max_completion_tokens=N)")
    parser.add_argument("--n", type=int, help="parámetro n (número de elecciones)")
    parser.add_argument("--stop", action="append", metavar="SEQ",
                        help="secuencia de parada, repetible")
    parser.add_argument("--presence-penalty", type=float, help="parámetro presence_penalty")
    parser.add_argument("--frequency-penalty", type=float, help="parámetro frequency_penalty")
    parser.add_argument("--seed", type=int, help="parámetro seed")
    parser.add_argument("--response-format", choices=["text", "json_object", "json_schema"],
                        default="text", help="formato de respuesta (por defecto: text)")
    parser.add_argument("--json-schema-file",
                        help="ruta al JSON Schema cuando --response-format=json_schema")
    parser.add_argument("--schema-name",
                        help="nombre del esquema (el del fichero si se omite)")
    parser.add_argument("--stream", action="store_true",
                        help="pide la respuesta en streaming (SSE); cada fragmento se emite "
                             "en stdout al recibirlo")
    parser.add_argument("--no-stream-options", action="store_true",
                        help="no enviar stream_options={include_usage:true} con --stream")
    parser.add_argument("--param", action="append", default=[], metavar="NOMBRE=VALOR",
                        help="parámetro adicional del cuerpo JSON (valor como JSON o cadena), "
                             "repetible")
    parser.add_argument("--header", action="append", default=[], metavar="NOMBRE:VALOR",
                        help="cabecera HTTP adicional, repetible")
    parser.add_argument("--timeout", type=float, default=60.0,
                        help="timeout en segundos de conexión y lectura (por defecto: 60)")
    parser.add_argument("--skip-models", action="store_true",
                        help="no consultar GET /models antes de la llamada de prueba")
    parser.add_argument("--insecure", action="store_true",
                        help="no verificar el certificado TLS del servidor")
    parser.add_argument("-v", "--verbose", action="store_true",
                        help="volcar por stderr el cuerpo de la petición y la respuesta cruda")
    return parser, parser.parse_args(argv)


def main() -> None:
    for stream in (sys.stdout, sys.stderr):
        if hasattr(stream, "reconfigure"):
            stream.reconfigure(encoding="utf-8", errors="replace")

    parser, args = parse_args(sys.argv[1:])

    if not args.baseurl.startswith(("http://", "https://")):
        parser.error("--baseurl debe empezar por http:// o https:// (recibido: "
                     f"{args.baseurl!r})")

    extra = parse_extra_params(args.param, parser)

    prompt = sys.stdin.buffer.read().decode("utf-8", "replace").rstrip("\r\n")
    if not prompt.strip():
        print("[error] stdin está vacío: el texto del mensaje se lee por stdin",
              file=sys.stderr)
        sys.exit(2)

    headers = build_headers(args, parser)

    payload = build_payload(args, prompt, extra)
    if args.verbose:
        log("req:  " + json.dumps(payload, ensure_ascii=False))
    url = f"{args.baseurl.rstrip('/')}/chat/completions"

    timeout = httpx.Timeout(args.timeout, connect=min(args.timeout, 10.0))
    try:
        with httpx.Client(timeout=timeout, verify=not args.insecure,
                          follow_redirects=True, headers=headers) as client:
            if not args.skip_models:
                check_models(client, args.baseurl)
            if args.stream:
                chat_stream(client, url, payload, headers, args.verbose)
            else:
                chat_non_stream(client, url, payload, headers, args.verbose)
    except KeyboardInterrupt:
        print("\n[error] interrupción del usuario", file=sys.stderr)
        sys.exit(130)


if __name__ == "__main__":
    main()
