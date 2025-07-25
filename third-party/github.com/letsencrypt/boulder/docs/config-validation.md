# Configuration Validation

We use a fork of https://github.com/go-playground/validator which can be found
at https://github.com/letsencrypt/validator. 

## Usage

By default Boulder validates config files for all components with a registered
validator. Validating a config file for a given component is as simple as
running the component directly:

```shell
$ ./bin/boulder-observer -config test/config-next/observer.yml
Error validating config file "test/config-next/observer.yml": Key: 'ObsConf.MonConfs[1].Kind' Error:Field validation for 'Kind' failed on the 'oneof' tag
```

or by running the `boulder` binary and passing the component name as a
subcommand:

```shell
$ ./bin/boulder boulder-observer -config test/config-next/observer.yml
Error validating config file "test/config-next/observer.yml": Key: 'ObsConf.MonConfs[1].Kind' Error:Field validation for 'Kind' failed on the 'oneof' tag
```

## Struct Tag Tips

You can find the full list of struct tags supported by the validator [here]
(https://pkg.go.dev/github.com/go-playground/validator/v10#section-documentation).
The following are some tips for struct tags that are commonly used in our
configuration files.

### `required`

The required tag means that the field is not allowed to take its zero value, or
equivalently, is not allowed to be omitted. Note that this does not validate
that slices or maps have contents, it simply guarantees that they are not nil.
For fields of those types, you should use min=1 or similar to ensure they are
not empty.

There are also "conditional" required tags, such as `required_with`,
`required_with_all`, `required_without`, `required_without_all`, and
`required_unless`. These behave exactly like the basic required tag, but only if
their conditional (usually the presence or absence of one or more other named
fields) is met.

### `omitempty`

The omitempty tag allows a field to be empty, or equivalently, to take its zero
value. If the field is omitted, none of the other validation tags on the field
will be enforced. This can be useful for tags like validate="omitempty,url", for
a field which is optional, but must be a URL if it is present.

The omitempty tag can be "overruled" by the various conditional required tags.
For example, a field with tag `validate="omitempty,url,required_with=Foo"` is
allowed to be empty when field Foo is not present, but if field Foo is present,
then this field must be present and must be a URL.

### `-`

Normally, config validation descends into all struct-type fields, recursively
validating their fields all the way down. Sometimes this can pose a problem,
when a nested struct declares one of its fields as required, but a parent struct
wants to treat the whole nested struct as optional. The "-" tag tells the
validation not to recurse, marking the tagged field as optional, and therefore
making all of its sub-fields optional as well. We use this tag for many config
duration and password file struct valued fields which are optional in some
configs but required in others.

### `structonly`

The structonly tag allows a struct valued field to be empty, or equivalently, to
take its zero value, if it's not "overruled" by various conditional tags. If the
field is omitted the recursive validation of the structs fields will be skipped.
This can be useful for tags like `validate:"required_without=Foo,structonly"`
for a struct valued field which is only required, and thus should only be
validated, if field `Foo` is not present.

### `min=1`, `gte=1`

These validate that the value of integer valued field is greater than zero and
that the length of the slice or map is greater than zero.

For instance, the following would be valid config for a slice valued field
tagged with `required`.
```json
{
  "foo": [],
}
```

But, only the following would be valid config for a slice valued field tagged
with `min=1`.
```json
{
  "foo": ["bar"],
}
```

### `len`

Same as `eq` (equal to) but can also be used to validate the length of the
strings.

### `hostname_port`

The
[docs](https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-HostPort)
for this tag are scant with detail, but it validates that the value is a valid
RFC 1123 hostname and port. It is used to validate many of the
`ListenAddress` and `DebugAddr` fields of our components.

#### Future Work

This tag is compatible with IPv4 addresses, but not IPv6 addresses. We should
consider fixing this in our fork of the validator.

### `dive`

This tag is used to validate the values of a slice or map. For instance, the
following would be valid config for a slice valued field (`[]string`) tagged
with `min=1,dive,oneof=bar baz`.

```json
{
  "foo": ["bar", "baz"],
}
```

Note that the `dive` tag introduces an order-dependence in writing tags: tags
that come before `dive` apply to the current field, while tags that come after
`dive` apply to the current field's child values. In the example above: `min=1`
applies to the length of the slice (`[]string`), while `oneof=bar baz` applies
to the value of each string in the slice.

We can also use `dive` to validate the values of a map. For instance, the
following would be valid config for a map valued field (`map[string]string`)
tagged with `min=1,dive,oneof=one two`.

```json
{
  "foo": {
    "bar": "one",
    "baz": "two"
  },
}
```

`dive` can also be invoked multiple times to validate the values of nested
slices or maps. For instance, the following would be valid config for a slice of
slice valued field (`[][]string`) tagged with `min=1,dive,min=2,dive,oneof=bar
baz`.

```json
{
  "foo": [
    ["bar", "baz"],
    ["baz", "bar"],
  ],
}
```

- `min=1` will be applied to the outer slice (`[]`).
- `min=2` will be applied to inner slice (`[]string`).
- `oneof=bar baz` will be applied to each string in the inner slice.

### `keys` and `endkeys`

These tags are used to validate the keys of a map. For instance, the following
would be valid config for a map valued field (`map[string]string`) tagged with
`min=1,dive,keys,eq=1|eq=2,endkeys,required`.

```json
{
  "foo": {
    "1": "bar",
    "2": "baz",
  },
}
```

- `min=1` will be applied to the map itself
- `eq=1|eq=2` will be applied to the map keys
- `required` will be applied to map values
