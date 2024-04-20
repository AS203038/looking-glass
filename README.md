# looking-glass
Just another looking glass software because all existing ones are either dead or ancient

# Demo
[AS203038](https://lg.as203038.net/) runs this as a daily driver.

# Tech Stack
The foundation is built on Golang and gRPC (ConnectRPC to be precise). This Golang codebase is responsible for any and all interactions with the routers through SSH, in future plans an embedded goBGPD is planned.

The WebUI is built using SvelteKit and served as static files embedded in the final Golang binary, all configurations for the UI are injected in runtime by autogenerating the env.js.

WebUI and Golang communicate using gRPC-Web through ConnectRPC's SDKs with eachother.

In future there will be a generic CLI client that can connect to arbitrary instances of this looking-glass allowing you to query multiple instances at once. Assuming anyone actually will run this other than us.

# Configuration
All config is done in a single YAML. It's probably not perfect but it works well enough for now.

Example config is supplied with all release builds but can also be found [Here](https://github.com/r0cket-net/looking-glass/blob/main/example.config.yaml).

# Scalability
The server is stateless. It will work well with multiple replicas and loadbalancing schemes as long as the loadbalancer can serve gRPC traffic (HTTP/2).

Router Listing is paginated and the UI will switch to a expandable list format grouped by locations when too many routers are supplied (>4 decided by dice roll).
However other outputs (such as BGP routes, traceroute, ...) is not yet paginated as it would break statelessnes of the backends.
You are able to request full bgp tables through the UI, Chrome will crash and Firefox will become a slideshow but the backend will happily serve it without any issues.

# Logging
HTTP Requests are logged using common Apache Access Log Format (without timestamp)

# Contributions
More than welcome! We 100% would love more router models - if you do not want to write the code yourself you can also give us ReadOnly access to your router/s and we write the models.
