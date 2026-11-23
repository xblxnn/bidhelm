# EXADS JSON Pop

Create a traffic source in the backoffice and use its generated `/source/{id}` URL.
Send a JSON POST with `Content-Type: application/json`:

```json
{
  "id": "example-request",
  "type": "popunder",
  "sub": "placement-123",
  "ip": "81.2.69.160",
  "ua": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36"
}
```

`ip` is the end user's address; proxy headers do not replace it. `sub` is a
required nonempty placement ID, either a string or a nonnegative JSON integer.
Whitespace around a string is trimmed; `"00123"` differs from `123`.

A winning campaign returns HTTP 200:

```json
{"bid":{"id":"example-request","url":"https://tracker.example/click?site=placement-123","btype":1,"value":0.13}}
```

`value` is CPM in USD. `bUrl` and a billing endpoint are not provided in this
release. Validate compatibility with your SSP before sending live traffic.

A valid request without a winner receives HTTP 204 with an empty body.
Malformed requests receive HTTP 400. An unready bidder returns HTTP 503.

## Targeting

An empty or disabled filter means All. Enabled filters must match together;
entries within one filter are alternatives. IP filters accept individual
IPv4/IPv6 addresses and CIDR networks. Placement filters accept one ID per line
in whitelist or blacklist mode. Disabling a filter preserves its saved entries.

A campaign with strictly narrower matching targeting takes priority over a
broader one. Remaining candidates compete on CPM; ties are random.

Use `{placement_id}` inside destination URL query values, for example:

```text
https://tracker.example/click?site={placement_id}
```

The placement is URL-encoded. The macro works independently of placement filters.
Changes apply after the saved and applied revisions match; no restart is needed.
