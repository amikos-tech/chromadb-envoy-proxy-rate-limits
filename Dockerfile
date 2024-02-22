FROM envoyproxy/envoy:v1.29-latest
#COPY --from=build-env /path/to/your_plugin.wasm /usr/local/bin/your_plugin.wasm
COPY ./hello.wasm /usr/local/bin/hello.wasm
COPY envoy.yaml /etc/envoy/envoy.yaml
CMD ["envoy", "-c", "/etc/envoy/envoy.yaml", "--log-level", "info"]
