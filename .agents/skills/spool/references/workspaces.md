# Multi-repo workspaces

A detached workspace groups several independently checked-out repository paths under one name in a
central registry stored outside any single repository (not inside `.spl`). Use it when a task spans
more than one repository.

```sh
spl workspace init ecommerce-platform
spl workspace attach --workspace ecommerce-platform ~/repos/order-service
spl workspace attach --workspace ecommerce-platform ~/repos/inventory-service
spl workspace list
spl workspace current
spl workspace detach ~/repos/inventory-service
```

A repository path can be attached to only one workspace at a time; `attach` on an already-attached
path fails rather than moving it. `current` resolves the registered workspace owning the current
working directory by longest matching attached path and fails when no workspace owns it.

Persist or clear which workspace future sessions should treat as active:

```sh
spl workspace use ecommerce-platform
spl workspace unset
```

`unset` always succeeds, including when no preference is set, so run it to recover from a stale
preference pointing at a workspace no longer in the registry.
