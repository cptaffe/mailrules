# Mail Rules

A simple IMAP client which continuously applies mail rules to your Inbox. Rules are defined in a file, for example `rules.txt`.

The language consists of semicolon-delimited rules, each of which is a predicate followed by an action. For example:

```
if from = "marketing@example.com" or subject = "Deal!" then move "Archive";
```

will move any emails from `marketing@example.com` or the subject `Deal!` to your archive.

The predicate is formed of:

- Field equivalence, `to = "someone@example.com"`
- Field regular expression matches, `to ~ "@example.com$"`
- Boolean operators `and`, `or` and `not`, `to ~ "@example.com$" and not to = "important@example.com"`

The action can be one of:

- Move the message to a new folder, `move "Archive"`
- Flag the message, optionally with a custom flag, `flag` or `flag "My Flag"`

Regular expressions provide a powerful matching mechanism, for example:

```
if to ~ "^marketing\+" then move "Marketing";
```

will move any email sent to custom addresses like `marketing+llbean@example.com` to the folder `Marketing`. Likewise the rule:

```
if from ~ "[@.]llbean.com$" then move "Marketing";
```

will move any email sent from an `llbean.com` email addres, or a subdomain of `llbean.com` (such as `info@e4.llbean.com`) to the folder `Marketing`.

## To Do

Possible future features:

- Reload configuration periodically
- Run at least once every x period
- Pass configuration via flags
- Support reading from mailboxes other than Inbox
  - Support rules that operate on these mailboxes (default to Inbox?)
- Support rules which can use data extracted from the predicate (e.g. regex capture groups can be templated into folder name)

## Creds

Use an app-specific password.

## Running

To run locally, first install `goyacc`:

```sh
; go install modernc.org/goyacc@latest
```

Then generate the parser:

```sh
go generate ./parse
```

Then compile and run `mailrules`:

```sh
go run . \
  --host=imap.mail.me.com:993 \
  --username=$(op read op://Personal/mailrules-icloud/username) \
  --password=$(op read op://Personal/mailrules-icloud/password) \
  --rules=rules.txt
```

To run within a docker image:

```sh
; docker build .
; docker run \
  -v $(pwd)/rules.txt:/etc/mailrules/rules.txt \
  sha256:1c2fdd985c6b3fae9e7c5613c249c1f3bfb2c3ae788bfabad3134e9b8ab83e86 \
  --host=imap.mail.me.com:993 \
  --username=$(op read op://Personal/mailrules-icloud/username) \
  --password=$(op read op://Personal/mailrules-icloud/password) \
  --rules=/etc/mailrules/rules.txt
```

## Deploy

1. Create a configmap containing our rules
   ```sh
   ; kubectl create configmap mailrules-rules --from-file=./rules.txt
   ```
2. Create a secret with login info
   ```sh
   ; kubectl create secret generic mailrules-icloud \
       --from-literal=username=$(op read op://Personal/mailrules-icloud/username) \
       --from-literal=password=$(op read op://Personal/mailrules-icloud/password)
   ```
3. Run `./deploy` to build the image and template the `deployment.yaml`
