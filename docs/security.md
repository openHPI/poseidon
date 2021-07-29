# Security configurations

## Authentication

⚠️ Don't use authentication without TLS enabled, as otherwise the token will be transmitted in clear text.

### Poseidon

⚠️ We encourage you to enable authentication for this API. If disabled, everyone with access to your API has also indirectly access to your Nomad cluster as this API uses it.

The API supports authentication via an HTTP header. To enable it, specify the `server.token` value in the `configuration.yaml` or the corresponding environment variable `POSEIDON_SERVER_TOKEN`.

Once configured, all requests to the API, except the `health` route require the configured token in the `X-Poseidon-Token` header.

An example `curl` command with the configured token being `SECRET` looks as follows:

```bash
$ curl -H "X-Poseidon-Token: SECRET" http://localhost:7200/api/v1/some-protected-route
```

### Nomad

⚠️ Enabling access control in the Nomad cluster is also recommended, to avoid having unauthorized actors perform unwanted actions in the cluster. Instructions on setting up the cluster appropriately can be found in [the Nomad documentation](https://learn.hashicorp.com/collections/nomad/access-control).

Afterwards, it is recommended to create a specific [Access Policy](https://learn.hashicorp.com/tutorials/nomad/access-control-policies?in=nomad/access-control) for Poseidon with the minimal set of capabilities it needs for operating the cluster. A non-minimal example with complete permissions can be found [here](docs/poseidon_policy.hcl). Poseidon requires a corresponding [Access Token](https://learn.hashicorp.com/tutorials/nomad/access-control-tokens?in=nomad/access-control) to send commands to Nomad. A Token looks like this:

```text
Accessor ID  = 463d3216-dc16-570f-380c-a48f5d26d955
Secret ID    = ea1ac4c5-892b-0bcc-9fc5-5faeb5273a13
Name         = Poseidon access token
Type         = client
Global       = false
Policies     = [poseidon]
Create Time  = 2021-07-26 12:45:11.437786378 +0000 UTC
Create Index = 246238
Modify Index = 246238
```

The `Secret ID` of the Token needs to be specified as the value of `nomad.token` value in the `configuration.yaml` or the corresponding environment variable `POSEIDON_NOMAD_TOKEN`. It may also be required for authentication in the Nomad Web UI and for using the Nomad CLI on the Nomad hosts (where the token can be specified via the `NOMAD_TOKEN` environment variable).

Once configured, all requests to the Nomad API automatically contain a `X-Nomad-Token` header containing the token.

⚠️ Make sure that no (overly permissive) `anonymous` access policy is present in the cluster after the policy for Poseidon has been added. Anyone can perform actions as specified by this special policy without authenticating!

## TLS

We highly encourage the use of TLS in this API to increase the security.

### Poseidon

To enable TLS, you need to create an appropriate certificate first. Then,
set `server.tls.active` or the corresponding environment variable to `true` and specify the `server.tls.certfile` and `server.tls.keyfile` options.

### Nomad

To enable TLS between Poseidon and Nomad, TLS needs to be first activated in Nomad. See [the Nomad documentation](https://learn.hashicorp.com/collections/nomad/transport-security) for a guideline on how to do that.

Afterwards, it is *required* to set the `nomad.tls.active` config option to `true`, as Nomad will no longer accept any connections over HTTP. To make sure the authenticity of the Nomad host can be validated, the `nomad.tls.cafile` option has to point to a certificate of the signing authority.

If using mutual TLS between Poseidon and Nomad is desired, the `nomad.tls.certfile` and `nomad.tls.keyfile` options can hold a client certificate. This certificate must be signed by the same CA as the certificates of the Nomad hosts. Note that mTLS can (and should) be enforced by Nomad in this case using the [verify_https_client](https://www.nomadproject.io/docs/configuration/tls#verify_https_client) configuration option.
