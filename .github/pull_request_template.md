## Contexte

<!-- Quel problème cette PR résout-elle ? Lien vers l'issue : Fixes #… -->

## Changements

<!-- L'essentiel, en quelques lignes. -->

## Comment j'ai testé

<!-- Tests ajoutés / commandes lancées / vérification manuelle (SMSH_MOCK=1). Captures d'écran pour l'UI, SANS donnée réelle. -->

## Liste de contrôle

- [ ] `gofmt -l .` ne signale rien, `go vet ./...` et `go test ./...` passent
- [ ] `npm run check` passe (si le front est touché)
- [ ] J'ai ajouté ou adapté des tests pour le comportement modifié
- [ ] Aucun secret, numéro ou message réel dans le code, les tests, les logs ou les captures
- [ ] La documentation (README, aide de la CLI) est à jour si nécessaire
- [ ] Pas de nouvelle dépendance, ou elle est justifiée ci-dessus
