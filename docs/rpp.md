# Reginald Plugin Protocol Version 0 Specification

This document describes the version 0 of the Reginald plugin protocol (also
_RPP_).

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
only client can send requests and the server can send responses, but in RPP the
server is allowed to send information to the client and the client may respond.
For example, this can be especially useful as the program benefits from
centralizing logging to the client. The server can send logging messages as
notifications to the client that uses its logger to print the messages. We still
call the main program the client and the plugin the server to keep the naming
clear.

## Reginald Plugin Protocol

The Reginald plugin protocol defines a set of JSON-RCP requests, responses,
notifications, and methods which are exchanged and executed using the protocol
described above.
