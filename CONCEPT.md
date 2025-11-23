# Commands

- init
  - erstellt observer yaml config, fragt basics ab, braucht aber age zum verschlüsseln
- deploy
  - rollt observer aus, erstellt Service und timer für den Service
  - SSH mit entsprechenden privlegien
  - muss age file, service, timer und config auf den Sercer kopieren, überschreibt existierende
- reconcile
  - wird im service als command genutzt
  - hat age file als pfad und den config pfad
  - findet Änderungen im repo und rolled sie aus

# Deamon Config


Config should be applied as "starting" point with `skuld deploy --host 127.0.0.1 --ssh-key id_rsa --ssh-user deployuser my-observer.yaml`
 - the age key should also be a parameter for secret encryption
 - If host is ommited localhost will be assumed and now SSH key/user is taken into account
 - If ssh key and user is ommited the command will try to ssh with only the host, make sure to configure the host in your ssh config correctly

The command will create the service and time defintion and deploy the yaml to the server incl service

```
apiVersion: skuld/v1alpha1
kind: Observer
metadata:
  name: DEAMON SERVICE NAME
  namespace: argocd // maybe replace with execution user?
spec:
  destination: /some/path // Should be the root path were the content from source is deployed to
  syncPolicy: [] // TBD - what could be useful for sync?
  source:
    path: //path in repo were all
    repoURL: //git repo url
    targetRevision: //revision to work on, could also be a TAG
```
