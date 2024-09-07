Just another looking glass software because all existing ones are either dead or ancient.

# Demo
[AS203038](https://lg.as203038.net/) uses this as a daily driver.

# Tech Stack
The foundation is built on Golang and gRPC (specifically ConnectRPC). This Golang codebase handles all interactions with the routers through SSH. Future plans include incorporating an embedded goBGPD.

The WebUI is built using SvelteKit and served as static files embedded in the final Golang binary. All UI configurations are injected at runtime by auto-generating the `env.js` file.

The WebUI and Golang communicate using gRPC-Web through ConnectRPC's SDKs.

There is also a generic gRPC client available for direct interaction with the backend, in case you prefer not to use the WebUI. It is released as the lg-cli artifact in the releases. If you want, you can add your LG instance to the [public_index.yaml](https://github.com/AS203038/looking-glass/blob/main/public_index.yaml) file and submit a PR.

# Configuration
All configuration is done in a single YAML file. It may not be perfect, but it works well enough for now.

An example config is included with all release builds and can also be found [here](https://github.com/AS203038/looking-glass/blob/main/example.config.yaml).

# Scalability
The server is stateless and can work well with multiple replicas and load-balancing schemes, as long as the load balancer can handle gRPC traffic (HTTP/2).

Router listing is paginated, and the UI switches to an expandable list format grouped by locations when there are too many routers (>4, determined by a dice roll). Large outputs, such as BGP routes and traceroute, are now paginated to prevent browser crashes. You can request full BGP tables through the UI without any issues (except for Firefox on Windows arm64).

# Logging
HTTP requests are logged using the common Apache Access Log Format (without timestamp).

Optionally, you can enable Sentry logging and tracing, which will be applied to both the frontend and backend.

# Contributions
Contributions are more than welcome! We would love to have more router models. If you don't want to write the code yourself, you can also give us read-only access to your router(s), and we will write the models.