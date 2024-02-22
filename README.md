# Chroma Auth and Limits Demo

This repo demonstrates the following capabilities:

- Static quotas enforcement with periodic quota refresh
- Tiered quotas enforcement - quotas are enforced based on the user tier
- Dynamic quotas enforcement with periodic quota refresh (do we need that one or can we rely on OPA/OPAL for that?)
- AuthZ with KC and OPA/OPAL - fine-grained access control

## Getting started

```bash
make build
docker compose up --build
```

Use `cURL` to send a request to the server:

```bash
curl -X POST --location "http://localhost:18000" \
    -H "Content-Type: application/json" \
    -d '{
          "embeddings": [
            [
              0.5,
              0.5
            ]
          ],
          "metadatas": [
            {
              "key1": "strting",
              "key2": "string1232222222222222222",
              "key3": 1,
              "key4": 1.1,
              "key5": true
            }
          ],
          "documents": [
            "document 1"
          ],
          "uris": null,
          "ids": [
            "doc-id-1"
          ]
        }'
```

## Roadmap

- [ ] Periodic refresh of static quotas - How can this be done with OPA and OPAL where quotas are stored as JSON in a
  repo?

## Learnings about Envoy plugin development

- Tinygo to be used to compile the plugin to WebAssembly
- Plugin lifecycle is decoupled from request lifecycle
- Plugin lifecycle can be used to communicate with external services for fetching data and updating
- Plugin context can be injected in each request context
- If you use internal direct_response listener filter than responses bypass the plugin and get returned instead of
  request waiting for plugin to finish (especially if external async call is made)


## Request Authorization Sequence

- Check headers - check if required headers are present. For example, API key, content type, etc.
- Authz (API key validity) - check if the API key is valid and returns user identity which includes attributes and permissions
- Static/Global Quotas - check if the user has exceeded his static quotas, like maxium number of documents per request, maximum number of fields, maximum length of a field, etc.
- Dynamic Quotas  - check if the user has exceeded his dynamic quotas, like overall storage, daily request limit etc.
- Rate Limit (Global) - 
- Rate Limit (Tiered) - tiered/user specific rate limits
- Authz Resource (Coarse-grained) - check if the user has access to the resource he is trying to access
- Authz Resource (Fine-grained) - check if the user has access to the specific action he is trying to perform on the resource


## References

**Plugins:**

- https://tufin.medium.com/extending-envoy-proxy-with-golang-webassembly-e51202809ba6
- https://github.com/tetratelabs/proxy-wasm-go-sdk/blob/main/examples/http_auth_random/envoy.yaml

**Rate Limiting:**

- https://github.com/envoyproxy/ratelimit/
- https://projectcontour.io/guides/global-rate-limiting/
- https://github.com/envoyproxy/data-plane-api/blob/main/envoy/service/ratelimit/v3/rls.proto - Envoy Ratelimit Proto spec

**OPA/OPAL:**

- https://www.openpolicyagent.org/docs/latest/policy-reference/