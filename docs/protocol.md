# Reginald Plugin Protocol Version 0 Specification

This document describes the version 0 of the Reginald plugin protocol (also
_RPP_). Plugins for Reginald are executables that Reginald invokes from the
configured plugin directory. Reginald uses the process’s standard input and
output pipes to communicate with the plugin using the Reginald plugin protocol
described in this document.

The protocol uses headers with JSON-RCP to call methods and send notifications.
If you have used the
[Language Server Protocol](https://microsoft.github.io/language-server-protocol/),
the Reginald plugin protocol should feel familiar to you. JSON-RCP and the
standard input and output pipes were chosen to make the plugin system in
Reginald language-agnostic. Virtually any programming language can be used as
long as it can read the standard input and write to standard output. While
Reginald doesn’t support communicating with plugins over other transport
methods, the protocol could be trivially extended to work even over network
connections.

This document uses [JSON Schema](https://json-schema.org/) to show examples of
the types used with JSON-RCP. While types in JSON and in many languages that use
it can be a bit different or even dynamic, the types in this document describe
the types that Reginald expects the JSON to decode to. They also describe the
types from which Reginald encodes the JSON.

The Reginald Plugin Protocol is loosely based on the
[Language Server Protocol](https://microsoft.github.io/language-server-protocol/)
by Microsoft. The Language Server Protocol is licensed under
[Creative Commons Attribution 4.0 International](https://creativecommons.org/licenses/by/4.0/).

## The Protocol

A message consists of a header part and a content part. The header and the
content are separated by `\r\n`.

### Header

The header consists of header fields. Each header field is comprised of a name
and a value. The name and the value are separated by ": " (a colon and a space).
The header field is terminated by `\r\n`. The header is terminated by an empty
line, i.e. `\r\n\r\n` after the last header. The header part is defined as
follows:

```text
<header part>     ::= <header fields> "\r\n"
<header fiels>    ::= <header field>
                    | <header field> <header fields>
<header field>    ::= <field name> ":" <optional whitespace> <field value> "\r\n"
<field name>      ::= <non-digit>
                    | <non-digit> <field-name>
<non-digit>       ::= <letter>
                    | "-"
<letter>          ::= "A" | "B" | "C" | "D" | "E" | "F" | "G" | "H" | "I" | "J"
                    | "K" | "L" | "M" | "N" | "O" | "P" | "Q" | "R" | "S" | "T"
                    | "U" | "V" | "W" | "X" | "Y" | "Z" | "a" | "b" | "c" | "d"
                    | "e" | "f" | "g" | "h" | "i" | "j" | "k" | "l" | "m" | "n"
                    | "o" | "p" | "q" | "r" | "s" | "t" | "u" | "v" | "w" | "x"
                    | "y" | "z"
<field value>     ::= any ASCII token except "\r" and "\n"
<optional whitespace> ::= ""
                        | " "
                        | " " <optional whitespace>
```

The header must be encoded in ASCII encoding.

The following header fields are supported:

| Header field name | Value type | Description                                                       |
| ----------------- | ---------- | ----------------------------------------------------------------- |
| Content-Length    | int        | The length of the content part in bytes. This header is required. |

### Content

The content part consists of the actual information of the message. It uses an
[extended](#json-rcp-extension) version of [JSON-RCP](https://www.jsonrpc.org)
2.0 to describe the messages. Each message is either a request, response, or
notification.

<!-- TODO: Include information on the supported encoding. -->

**Example:**

```text
Content-Length: ...\r\n
\r\n
{
 "jsonrpc": "2.0",
 "id": 1,
 "method": "handshake",
 "params": {
  ...
 }
}
```

#### JSON-RCP Extension

The only extension used by the JSON-RCP specification the RPP uses is that both
the client and the server can send both requests and responses. In JSON-RCP 2.0,
only client can send requests and the server can send responses, but in the RPP
the server is allowed to send notification to the client. For example, this can
be especially useful as the program benefits from centralizing logging to the
client. The server can send logging messages as notifications to the client that
uses its logger to print the messages. We still call the main program the client
and the plugin the server to keep the naming clear.

### Base Types

This document uses [JSON Schema](https://json-schema.org/) to formally document
the expected types and forms of the protocol. Each type is also provided as
pseudo-code to make them easier to read. Please note that the RPP does not at
this time provide an actual JSON Schema file.

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/reginald-project/reginald/blob/main/docs/protocol.md",
  "title": "Reginald Plugin Protocol",
  "type": "object"
}
```

#### Message

Every message sent using the RPP must be a message as defined by JSON-RCP. The
RPP always uses `"2.0"` as the JSON-RCP version given as the `jsonrcp` member.

```json
{
  "$id": "#message",
  "title": "Message",
  "type": "object",
  "properties": {
    "jsonrpc": {
      "type": "string"
    }
  },
  "required": ["jsonrpc"]
}
```

```typescript
interface Message {
  jsonrpc: string;
}
```

#### Request

A request from the client to the server. The server must respond to a request
with a [Response](#response).

```json
{
  "$id": "#request",
  "title": "Request",
  "type": "object",
  "allOf": {
    "$ref": "#message"
  },
  "properties": {
    "id": {
      "type": ["integer", "string"]
    },
    "method": {
      "type": "string"
    },
    "params": {
      "type": ["array", "object", "null"]
    }
  },
  "required": ["id", "method"]
}
```

```typescript
interface Request extends Message {
  id: int | string;
  method: string;
  params: any[] | object | null;
}
```

#### Response

A response is sent from the server to the client as the result of a
[Request](#request). The `id` must be the same as the `id` in the request unless
there was an error detecting the `id` from the request. If that is the case, the
`id` must be `null`. The `result` must be present on success and omitted on
error. The `error` must be present on error and omitted on success.

```json
{
  "$id": "#response",
  "title": "Response",
  "type": "object",
  "allOf": {
    "$ref": "#message"
  },
  "properties": {
    "id": {
      "type": ["integer", "string", "null"]
    },
    "result": {},
    "error": {
      "$ref": "#error"
    }
  }
}
```

```typescript
interface Response extends Message {
  id: int | string | null;
  result: any | null;
  error: Error | null;
}
```

#### Error

Error is the `error` object in [Response](#response) if it is not successful.
The member `code` indicates the type of the error that occured and the `message`
contains the error message. The member `data` may contain additional information
on the error and it may be omitted. The error codes that are currently used are
as follows:

| Code   | Message          | Meaning                                                                                              |
| ------ | ---------------- | ---------------------------------------------------------------------------------------------------- |
| -32700 | Parse error      | Invalid JSON was received by the server. An error occurred on the server while parsing the JSON text |
| -32600 | Invalid Request  | The JSON sent is not a valid Request object.                                                         |
| -32601 | Method not found | The method does not exist / is not available.                                                        |
| -32602 | Invalid params   | Invalid method parameter(s).                                                                         |
| -32603 | Internal error   | Internal JSON-RPC error.                                                                             |

```json
{
  "$id": "#error",
  "title": "Error",
  "type": "object",
  "properties": {
    "code": {
      "type": "integer"
    },
    "message": {
      "type": "string"
    },
    "data": {}
  },
  "required": ["code", "message"]
}
```

```typescript
interface Error {
  code: int;
  message: string;
  data: any | null;
}
```

#### Notification

A processed notification message must not send a response back.

```json
{
  "$id": "#notification",
  "title": "Notification",
  "type": "object",
  "allOf": [
    {
      "$ref": "#message"
    }
  ],
  "properties": {
    "method": {
      "type": "string"
    },
    "params": {
      "type": ["array", "object", "null"]
    }
  },
  "required": ["method"]
}
```

```typescript
interface Notification extends Message {
  method: string;
  params: any[] | object | null;
}
```

## Reginald Plugin Protocol

The Reginald plugin protocol defines a set of JSON-RCP requests, responses,
notifications, and methods which are exchanged and executed using the protocol
described above. This sections describes the methods and JSON structures used in
the protocol to run the actual capabilities of the plugins. Please note that due
to different requirements of the programming languages a `null` value in the
JSON and in the types described here effectively means that the value is
omitted. For example, in JSON-RCP 2.0 specification the ID should be omitted if
a request is a notification but as Go is a statically-typed language, the ID
will be `null` (or `nil`) if it omitted.
