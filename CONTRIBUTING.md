# Contribuer à Smsh

Merci de votre intérêt ! Smsh est un outil qui manipule des identifiants OVH et envoie de vrais SMS (donc de l'argent) : les contributions sont relues avec soin, surtout sur la sécurité et la fiabilité des envois.

## Avant de commencer

- **Bug ou idée** : ouvrez d'abord une *issue* pour en discuter, surtout pour une fonctionnalité. Une PR qui arrive sans discussion préalable risque d'être refusée.
- **Faille de sécurité** : ne l'ouvrez **pas** en issue publique, voir [SECURITY.md](SECURITY.md).

## Mettre en place l'environnement

Prérequis et commandes : voir le [README](README.md#démarrage) (Go, Node 22, Wails v2).

```powershell
wails dev                       # développement avec rechargement
$env:SMSH_MOCK = "1"            # faux OVH : aucune clé ni SMS réel nécessaire
```

N'utilisez **jamais** un vrai compte OVH pour développer ou tester : le mode `SMSH_MOCK=1` et les tests avec le faux client suffisent presque toujours.

## Proposer une modification

1. *Fork*, puis une branche dédiée par sujet (`fix/…`, `feat/…`), depuis `main` à jour.
2. Changements petits et ciblés : une PR = un sujet. Pas de refactorisation mêlée à une correction.
3. Vérifiez en local, comme le fait la CI :
   ```powershell
   gofmt -l .                      # ne doit rien afficher
   go vet ./...
   go test ./...
   cd frontend; npm run check
   ```
4. Ouvrez la PR vers `main` en remplissant le modèle. Tant que la CI n'est pas verte, la PR ne peut pas être fusionnée.

## Règles de code

- **Pas de logique métier dans le front, dans `app/` ou dans la CLI** : ces couches appellent `internal/service`. Les règles vont dans `internal/`.
- **Secrets** : jamais en clair sur disque, jamais dans les logs ni dans les messages d'erreur, jamais renvoyés au front (valeurs masquées uniquement). Aucune clé réelle dans le code, les tests, les captures ou les issues.
- **Tests obligatoires** pour tout changement de comportement dans le moteur de campagne, le wrapper OVH, le coffre de clés ou l'historique. Un bug corrigé = un test qui échouait avant.
- Le wrapper OVH reste derrière l'interface `ovh.Client` ; les tests passent par le mock, sans réseau.
- Les erreurs affichées à l'utilisateur sont **claires et en français** (voir `ovh.Error` et `service.Describe`).
- Pas de nouvelle dépendance sans justification dans la PR (taille du binaire, licence compatible, maintenance). Pas de CGO.
- L'interface doit rester utilisable au clavier et lisible dans les thèmes clair et sombre.

## Relecture et fusion

Une PR est fusionnée quand **toutes** ces conditions sont remplies :

- [ ] la CI est verte (tests, vérification du front, builds Windows / Linux / macOS) ;
- [ ] au moins une approbation d'un mainteneur, et les commentaires sont résolus ;
- [ ] la branche est à jour avec `main` ;
- [ ] le modèle de PR est rempli, avec ce qui a été testé (et comment) ;
- [ ] pas de secret, de donnée personnelle (numéros, messages réels) ni de fichier généré inutile ;
- [ ] la documentation (README, aide de la CLI) est mise à jour si le comportement change.

Les mainteneurs fusionnent en **squash** (un commit propre par PR, titre à l'impératif, ex. « Corrige le décompte des envoyés d'un batch »). Personne ne pousse directement sur `main`, mainteneurs compris.

Les PR de sécurité, ou qui touchent au coffre de clés, à l'envoi de SMS ou à la suppression chez OVH, demandent une relecture particulièrement attentive et peuvent être refusées si elles manquent de tests.

## Publier une version (mainteneurs)

Les versions sont créées par un tag `vX.Y.Z` sur `main` : la CI construit et publie les binaires. Voir la section « Builds et publication » du README.

## Licence

En contribuant, vous acceptez que votre contribution soit publiée sous la licence du projet (voir le fichier `LICENSE`).
