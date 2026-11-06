# Smsh

> Prononcé « Smash ». Application de bureau pour piloter les API SMS d'OVH : envoi unitaire, campagnes depuis un CSV, historique.

Outil privé, **GUI d'abord** : tout se fait depuis l'interface, y compris la configuration initiale. Un CLI léger complétera l'ensemble en réutilisant le même cœur métier.

- **Go + [Wails v2](https://wails.io)**, front **Svelte 5 + TypeScript** : un seul binaire (~16 Mo), aucun runtime à installer.
- Cible principale **Windows 10/11** ; code portable Linux et macOS.
- Clés OVH stockées dans le **coffre du système**, jamais en clair sur disque, ni dans les logs, ni renvoyées à l'interface.

> **Sources officielles** : uniquement le dépôt <https://github.com/next-vertices/smsh> et les sites de Next Vertices. Tout autre binaire ou paquet est non officiel (voir [TRADEMARKS.md](TRADEMARKS.md)). Les contributions sont les bienvenues (voir [CONTRIBUTING.md](CONTRIBUTING.md)).

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
- [Node.js](https://nodejs.org/) 22+ et npm
- CLI Wails v2 : `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`
- Dépendances système, selon la plateforme :

| Plateforme | À installer |
|---|---|
| Windows 10/11 | WebView2 (déjà présent) ; un terminal PowerShell |
| Debian / Ubuntu | `sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev` |
| Fedora | `sudo dnf install gcc-c++ pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel` |
| Arch / Manjaro | `sudo pacman -S base-devel gtk3 webkit2gtk-4.1` |
| macOS | outils de ligne de commande Xcode : `xcode-select --install` |

Lancez `wails doctor` pour vérifier que tout est en place.

### Développer

```powershell
wails dev                 # rechargement à chaud (Linux : ajouter -tags webkit2_41)
go test ./...             # tests Go
cd frontend; npm install; npm run check   # types Svelte + cohérence des traductions
```

### Compiler

Un seul exécutable sert d'application et de ligne de commande. Le résultat est dans `build/bin/`.

| Plateforme | Commande | Résultat |
|---|---|---|
| Windows | `wails build -ldflags "-s -w"` | `build\bin\smsh.exe` |
| Linux | `wails build -tags webkit2_41 -ldflags "-s -w"` | `build/bin/smsh` |
| macOS | `wails build -platform darwin/universal -ldflags "-s -w"` | `build/bin/smsh.app` (Intel + Apple Silicon) |

- `wails build` compile d'abord le front (`npm install` + `npm run build`), puis le binaire Go. Aucune étape manuelle.
- Pas de compilation croisée : le moteur web dépend du système, chaque plateforme se compile sur elle-même (la CI de GitHub s'en charge, voir plus bas).
- `-ldflags "-s -w"` allège le binaire. Pour y inscrire un numéro de version affiché par `smsh --version` :
  `wails build -ldflags "-s -w -X smsh/internal/cli.Version=1.2.3"`.
- Nettoyer avant de recompiler : ajouter `-clean`.
- Linux : `-tags webkit2_41` est nécessaire avec WebKitGTK 4.1 (Debian 12+, Ubuntu 22.04+, Fedora 38+, Arch). Avec l'ancienne WebKitGTK 4.0, retirer ce tag.
- Binaire seul sans l'interface (`go build ./...` suffit pour vérifier la compilation Go) : `main.go` embarque `frontend/dist`, compilez donc le front avant (`cd frontend; npm run build`).

**Dépannage**
- `no matching files found` pour `frontend/dist` : le front n'est pas compilé. Lancez `cd frontend; npm install; npm run build`, ou `wails build`.
- Windows, « le fichier est utilisé par un autre processus » pendant la compilation : un antivirus ou une instance de `smsh.exe` ouverte. Fermez l'application, relancez, ou excluez le dossier du projet et `%TEMP%` de l'antivirus.
- `Package gtk+-3.0 was not found` (Linux) : installez les paquets du tableau ci-dessus.

### Mode démo (sans compte OVH)
Un faux OVH et un coffre en mémoire remplacent l'API : l'onboarding accepte n'importe quelles valeurs, et tout est simulé. Rien n'est écrit dans votre configuration.

```powershell
$env:SMSH_MOCK = "1"; .\build\bin\smsh.exe
```

## Ligne de commande

Le même exécutable sert d'application et de CLI : `smsh.exe` sans argument ouvre la fenêtre, `smsh.exe <commande>` s'exécute dans le terminal. Il partage les comptes, le coffre de clés et la base locale de l'application : un compte configuré dans l'interface est immédiatement utilisable. Les comptes se créent et se modifient dans l'application.

```powershell
.\build\bin\smsh.exe status
.\build\bin\smsh.exe send 0612345678 -m "Bonjour" --dry-run
```

| Commande | Rôle |
|---|---|
| `smsh status` | teste la connexion, affiche solde et expéditeurs |
| `smsh account ls` / `account use <id\|nom>` | comptes enregistrés (secrets masqués), compte actif |
| `smsh send <num>... -m "texte"` | envoi unitaire (5 numéros max), coût affiché avant confirmation |
| `smsh sync [--days 30]` | met à jour le cache local de l'historique |
| `smsh ls [filtres] [--sync]` | liste des SMS (`--to`, `--status`, `--from`, `--until`, `-q`, `--tag`, `-n`) |
| `smsh rm <id>...` ou `rm --match <filtres>` | supprime chez OVH et dans le cache |
| `smsh campaign run <fichier.csv> -m "Bonjour {prenom}"` | simule puis lance (`--dry-run`, `--mode batch\|local`, `--rate`, `--at`, `--phone-col`) |
| `smsh campaign ls` / `show <id> [--recipients]` | suivi |
| `smsh campaign resume\|retry\|cancel\|rm <id>` | reprise, relance des échecs, annulation, suppression |

Points à connaître :
- `--json` sur toutes les commandes (erreurs comprises : `{"error":{"kind","message"}}` sur la sortie standard).
- Les actions qui envoient ou suppriment demandent une confirmation ; `--yes` la remplace (obligatoire sans terminal).
- Codes de sortie : `0` succès, `1` erreur, `2` erreur d'usage. Une campagne avec des échecs ou en pause sort en `1`.
- Coffre à passphrase : variable `SMSH_PASSPHRASE`, sinon saisie masquée.
- Campagne **locale** : exécutée par le terminal qui l'a lancée. `Ctrl-C` la met en pause, `smsh campaign resume <id>` la reprend. Une campagne **batch** est traitée par OVH et continue sans le terminal (`--wait` pour l'attendre).
- `smsh.exe` étant une application « fenêtre », `cmd` et PowerShell ne l'attendent pas : pour récupérer le code de sortie ou chaîner des commandes, utilisez `Start-Process -Wait -NoNewWindow` / `start /wait`, ou redirigez la sortie (`.\smsh.exe ls --json | Out-String`).
- `SMSH_LANG=fr|en` : langue de la CLI (sinon celle du système).
- `SMSH_MOCK=1` : simulation sans réseau ; `SMSH_MOCK_DIR=<dossier>` conserve les campagnes entre deux appels.

## Langues (i18n)

L'interface, les messages du cœur (erreurs, avertissements) et la CLI sont traduits. Langue par défaut : **français (fr-FR)**. Au démarrage, la langue du système est reprise ; si elle n'est pas française, l'application passe en **anglais britannique (en-GB)**. Le choix manuel (Paramètres → Langue : Automatique / Français / English) est conservé. La CLI suit la langue du système, ou `SMSH_LANG=fr|en`.

| Fichiers | Contenu |
|---|---|
| `frontend/src/locales/<langue>.json` | textes de l'interface (clés plates `section.nom`) |
| `internal/i18n/locales/<langue>.json` | messages du cœur Go et de la CLI |

Les paramètres s'écrivent `{nom}` ; les pluriels utilisent les suffixes `_one` / `_other` avec le paramètre `count`. Les formats de date, de nombre et de monnaie suivent la langue.

**Ajouter une langue** : créer les deux fichiers JSON, déclarer la langue dans `frontend/src/lib/i18n.svelte.ts` (`LANGS`, `normalizeLang`) et dans `internal/i18n/i18n.go` (`Supported`, `Normalize`), puis ajouter l'entrée dans le sélecteur des Paramètres. `npm run check` (clés manquantes ou inutilisées, paramètres incohérents) et `go test ./internal/i18n` vérifient la cohérence des catalogues.

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
internal/cli/        CLI cobra, embarquée dans le même exécutable que l'application
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

## Builds et publication (GitHub Actions)

Le workflow [`.github/workflows/build.yml`](.github/workflows/build.yml) s'exécute à chaque push sur `main`, chaque pull request et sur demande : il lance `go vet`, `go test` et `npm run check`, puis compile sur trois systèmes (Wails dépend du moteur web du système : pas de compilation croisée).

| Plateforme | Artefacts |
|---|---|
| Windows | `smsh_<version>_windows_amd64.zip` (contient `smsh.exe`) |
| Linux (Debian/Ubuntu, Fedora, Arch/Manjaro…) | `.deb`, `.rpm`, `.pkg.tar.zst` (via nfpm, dépendances GTK 3 + WebKitGTK 4.1 déclarées) et `.tar.gz` du binaire |
| macOS | `smsh_<version>_macos_universal.zip` (`smsh.app`, Intel + Apple Silicon) |

- **Release** : poussez un tag `vX.Y.Z` (`git tag v1.0.0; git push origin v1.0.0`). Les artefacts sont publiés avec `SHA256SUMS.txt` et des notes générées. Le numéro de version est injecté dans `smsh --version`.
- Hors tag, les binaires sont téléchargeables depuis l'onglet *Actions* de l'exécution (artefacts), version `0.0.0-dev`.
- Linux : compilé sur Ubuntu 22.04 avec `-tags webkit2_41` ; il faut WebKitGTK 4.1 (Debian 12+, Ubuntu 22.04+, Fedora 38+, Arch à jour). Le paquet installe aussi l'entrée de menu et l'icône.
- macOS : l'application n'est ni signée ni notarisée ; au premier lancement : clic droit > Ouvrir, ou `xattr -dr com.apple.quarantine smsh.app`. Windows peut afficher SmartScreen (exécutable non signé).

## Limites connues

- **Statuts d'historique** déduits des dates (pas de statut « Échec » explicite d'OVH).
- **Pays inconnus** : la table indicatif → pays couvre ~100 destinations courantes ; un numéro d'un autre pays n'est pas chiffré (signalé). `+1` est rattaché aux États-Unis.
- **Segments avec variables** : calculés localement ; la mention STOP ajoutée par OVH n'est pas comptée.
- **Envoi différé en mode Personnalisé** : transmis à chaque SMS, à valider sur un compte réel.
- **Premier chargement de l'historique** : un compte très actif (plusieurs milliers de SMS) demande un téléchargement d'un SMS par requête ; limitez la période.

## Feuille de route

- [ ] Export CSV (campagnes, historique) depuis les Paramètres
- [x] CLI : `send`, `ls`, `rm`, `sync`, `status`, `account`, `campaign`, sortie `--json`
- [ ] Statut d'échec explicite dans l'historique (selon les codes OVH réels)

## Licence

Code sous licence [PolyForm Noncommercial 1.0.0](LICENSE) : vous pouvez l'utiliser, l'étudier, le modifier et proposer des contributions pour un usage **non commercial**. La **vente**, l'usage commercial et la diffusion d'une version concurrente sont interdits sans l'accord de Next Vertices. Le nom « Smsh » et le logo sont réservés (voir [TRADEMARKS.md](TRADEMARKS.md)) ; les seules sources officielles sont celles de Next Vertices.
