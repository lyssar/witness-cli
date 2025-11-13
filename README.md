<p align="center">
  <img src="assets/logo.png" alt="Skuld CLI Logo" width="320" />
</p>

<h1 align="center">Skuld CLI</h1>
<h3 align="center">the future’s watcher for your fleet</h3>

---

## Prepare AGE Key

```
AGE_KEY_FILE="/path/to/key.age"
age-keygen -o ${AGE_KEY_FILE}

# if you use 1password push the entry to save it
op item create --category="API_Credential" --title "API Credentials ($(date +%d/%m/%Y))" "[file]=${AGE_KEY_FILE}"
```

## Testing

```
export manifest="my-manifest.yaml"
export ageKeyFile="my-key.age"

yq -r '.spec.secrets.SECRET_NAME' ${manifest} | base64 -d | age -d -i ${ageKeyFile} | tee secret.enc

nano secret.enc
```

Quick start
- Build: `go build .`
- Run: `./skuldcli` → prints "Hello from SkuldCli!"

