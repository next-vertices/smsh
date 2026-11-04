# Smsh

> Prononcé « Smash ». Application de bureau pour piloter les API SMS d'OVH : envoi unitaire, campagnes depuis un CSV, historique.

Outil privé, **GUI d'abord** : tout se fait depuis l'interface, y compris la configuration initiale. Un CLI léger complétera l'ensemble en réutilisant le même cœur métier.

- **Go + [Wails v2](https://wails.io)**, front **Svelte 5 + TypeScript** : un seul binaire (~16 Mo), aucun runtime à installer.
- Cible principale **Windows 10/11** ; code portable Linux et macOS.
- Clés OVH stockées dans le **coffre du système**, jamais en clair sur disque, ni dans les logs, ni renvoyées à l'interface.

## Fonctionnalités

| Domaine | État | Détail |
|---|---|---|
| **Onboarding** | ✅ | Clés OVH, serviceName, « Tester la connexion », plusieurs comptes |
| **Envoyer** | ✅ | 1 à 5 destinataires, compteur de caractères/segments, expéditeur, envoi différé, mention STOP, coût estimé avant envoi |
| **Campagnes** | ✅ | Import CSV, templates `{variable}`, simulation (dry-run), coût par pays, progression en temps réel, pause / reprise / annulation |
| **Historique** | ✅ | Cache local, recherche et filtres (date, destinataire, statut, expéditeur, campagne), détail, suppression unitaire ou en lot |
| **Paramètres** | 🟡 | Comptes et thème clair/sombre faits ; **export CSV** à venir |
| **CLI** (`smsh send / ls / rm / campaign run`) | ⏳ | Prévu, sortie `--json`, même coffre que le GUI |

### Envoyer
- Limité à **5 destinataires** (règle dans le cœur Go ; pour plus, utilisez une campagne).
- Numéros saisis librement (`06 12 34 56 78`, `+33…`, `0033…`), normalisés en E.164 et dédoublonnés.
- **Coût réel avant envoi** : segments via `POST /sms/estimate`, tarifs par pays via `/sms/rates/destinations` et `/sms/rates/packs`. Les numéros étrangers sont chiffrés au bon tarif (ex. 1 crédit en France, 0,35 aux États-Unis).
- Une confirmation est demandée avant chaque envoi ; `Ctrl`+`Entrée` valide le formulaire.

### Campagnes
Deux modes, choisis selon le besoin :

| | **Batch OVH** | **Personnalisé** |
|---|---|---|
| Message | Identique pour tous | Variables `{colonne}` par destinataire |
| Exécution | Côté OVH (continue application fermée) | Moteur local : un SMS par destinataire, l'application doit rester ouverte |
| Débit | Géré par OVH | Limité (3 SMS/s par défaut, réglable) |
| Pause / reprise | Non (annulation possible si différé) | Oui, plus relance des échecs |
| Suivi | Statistiques OVH (délivrés, échecs, STOP) | Statut par destinataire, persisté |

Un message contenant des variables impose le mode Personnalisé.

**Flux** : choisir un CSV → choisir la colonne des numéros → rédiger le message → **simulation** (messages rendus, segments, coût et durée estimés, problèmes bloquants) → lancer.

**CSV** : la première ligne contient les en-têtes ; séparateur `;`, `,` ou tabulation ; UTF-8 ou Windows-1252 (export Excel FR). Les numéros invalides sont listés et ignorés, les doublons éliminés.

**Robustesse du moteur local** : l'état est écrit en base après chaque SMS. Une campagne interrompue (fermeture de l'application) repasse en pause et reprend **sans doublon**. Rate limit ou erreur réseau : nouvelles tentatives avec attente croissante, puis pause. Crédits insuffisants ou clés refusées : pause immédiate avec le message d'erreur. Numéro refusé : échec du destinataire, la campagne continue.

### Historique
- Synchronisation incrémentale avec OVH (période au choix : 7, 30 ou 90 jours), **les SMS les plus récents d'abord**, avec progression et bouton d'arrêt.
- Les périodes trop chargées sont découpées automatiquement quand OVH refuse la liste (« narrow your query filter »).
- La recherche et les filtres s'exécutent sur le cache SQLite local : instantanés.
- Statuts déduits des dates renvoyées par OVH : *Délivré* (accusé reçu), *Envoyé* (pas d'accusé), *Programmé*. Les codes bruts `deliveryReceipt` et `ptt` sont visibles dans le détail.

## Démarrage

### Prérequis
- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+ et npm
- WebView2 (déjà présent sur Windows 10/11)
- CLI Wails : `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Commandes

```powershell
# Développement avec rechargement à chaud
wails dev

# Build de production (binaire léger) -> build\bin\smsh.exe
wails build -ldflags "-s -w"

# Tests Go
go test ./...

# Vérification du front
cd frontend; npm run check
```

### Mode démo (sans compte OVH)
Un faux OVH et un coffre en mémoire remplacent l'API : l'onboarding accepte n'importe quelles valeurs, et tout est simulé. Rien n'est écrit dans votre configuration.

```powershell
$env:SMSH_MOCK = "1"; .\build\bin\smsh.exe
```

## Configuration OVH

Créez un token sur la page de création de token de votre région :
**https://eu.api.ovh.com/createToken/** (`ca.api.ovh.com`, `api.us.ovhcloud.com` selon l'endpoint). Elle fournit ensemble l'**application key**, l'**application secret** et le **consumer key**, à coller dans Smsh. Choisissez une validité « Unlimited » pour éviter l'expiration.

### Droits à accorder

| Méthode | Chemin | Usage |
|---|---|---|
| GET | `/sms` | lister les services |
| GET | `/sms/*` | compte, crédits, expéditeurs, historique, batches |
| POST | `/sms/*` | envoi, estimation, campagnes, annulation d'un batch |
| DELETE | `/sms/*` | suppression d'historique |

Les endpoints de tarifs (`/sms/rates/*`) sont publics : aucun droit à ajouter.

> **Les droits d'un consumer key sont figés à sa création.** Si vous modifiez les droits, générez un **nouveau** token et remplacez les trois valeurs dans Smsh.

Erreurs fréquentes :
- `This application key is invalid` : l'application key envoyée n'est pas reconnue par OVH. L'écran affiche la longueur reçue de chaque clé (attendu 16 / 32 / 32).
- `This call has not been granted` : les clés sont valides mais le consumer key n'a pas les droits ci-dessus.

## Stockage des données

Dossier de configuration (`%AppData%\smsh` sous Windows, `~/.config/smsh` sous Linux, `~/Library/Application Support/smsh` sous macOS) :

| Fichier | Contenu |
|---|---|
| `accounts.json` | Comptes (nom, serviceName, endpoint, compte actif). **Aucun secret.** |
| `smsh.db` | Base SQLite : campagnes, destinataires, cache de l'historique |
| `secrets.age` | Uniquement si le coffre du système est indisponible (voir ci-dessous) |

**Secrets** : stockés dans le coffre du système (Credential Manager, Keychain, Secret Service) via `go-keyring`. S'il n'est pas disponible, repli sur un **fichier chiffré par passphrase** ([age](https://age-encryption.org), scrypt), demandée via une modale. La passphrase ne peut pas être récupérée en cas d'oubli.

Variable d'environnement `SMSH_VAULT=age|keyring` : force le choix du coffre.

## Architecture

```
main.go              Point d'entrée Wails
app/                 Méthodes exposées au front (fines, sans logique métier)
internal/
  service/           Orchestration : creds + ovh + règles SMS (cœur partagé GUI/CLI)
  ovh/               Wrapper API OVH : interface, implémentation go-ovh, mock, erreurs typées
  creds/             Comptes et secrets (keyring / age)
  campaign/          Import CSV, templates, moteur d'envoi (débit, pause/reprise)
  sms/               Règles pures : encodage GSM-7/UCS-2, segments, numéros, pays
  store/             SQLite pur Go (modernc.org/sqlite, sans CGO)
frontend/            UI Svelte + TypeScript
cmd/smsh/            CLI (à venir)
```

Principes :
- **Aucune logique métier dans le front, Wails ou le CLI** : ces couches appellent `internal/service`.
- Le wrapper OVH est une **interface** (`ovh.Client`) avec implémentation réelle et mock : tout le cœur est testable sans réseau.
- Les erreurs OVH sont **classées** (`credentials`, `rate_limit`, `insufficient_credits`, `invalid_receiver`, `too_many_results`, `network`…) et affichées clairement dans l'UI. Elles traversent Wails en JSON `{kind, message}`.
- Pas de CGO : build croisé simple, binaire unique.

## Tests

`go test ./...` couvre notamment :
- le wrapper OVH (corps des requêtes, classification des erreurs, estimation, tarifs, batches, historique) avec un faux appelant HTTP ;
- le moteur de campagne (envoi complet, pause/reprise sans doublon, annulation, rate limit, crédits insuffisants, cadence) ;
- les coffres de secrets (aucun secret en clair sur disque, passphrase) ;
- la synchronisation et la recherche d'historique, la suppression en lot, le découpage de période.

Le détecteur de concurrence (`-race`) nécessite un compilateur C ; il n'est pas utilisé ici.

## Limites connues

- **Statuts d'historique** déduits des dates (pas de statut « Échec » explicite d'OVH).
- **Pays inconnus** : la table indicatif → pays couvre ~100 destinations courantes ; un numéro d'un autre pays n'est pas chiffré (signalé). `+1` est rattaché aux États-Unis.
- **Segments avec variables** : calculés localement ; la mention STOP ajoutée par OVH n'est pas comptée.
- **Envoi différé en mode Personnalisé** : transmis à chaque SMS, à valider sur un compte réel.
- **Premier chargement de l'historique** : un compte très actif (plusieurs milliers de SMS) demande un téléchargement d'un SMS par requête ; limitez la période.

## Feuille de route

- [ ] Export CSV (campagnes, historique) depuis les Paramètres
- [ ] CLI : `smsh send`, `smsh ls`, `smsh rm`, `smsh campaign run`, sortie `--json`
- [ ] Statut d'échec explicite dans l'historique (selon les codes OVH réels)
