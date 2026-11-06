# Politique de sécurité

Smsh manipule des clés d'API OVH et envoie des SMS : une faille peut avoir des conséquences financières. Merci de la signaler de façon responsable.

## Signaler une vulnérabilité

**N'ouvrez pas d'issue publique.** Utilisez le signalement privé de GitHub : onglet **Security** du dépôt, puis **Report a vulnerability**.

Indiquez si possible : la version, le système, les étapes pour reproduire et l'impact. Ne joignez jamais de vraies clés d'API.

Nous nous engageons à accuser réception sous quelques jours, à vous tenir informé de l'avancement et à vous créditer dans les notes de version si vous le souhaitez.

## Versions prises en charge

Seule la dernière version publiée reçoit des correctifs de sécurité.

## Ce qui nous intéresse en priorité

- fuite ou écriture en clair d'un secret (disque, logs, interface, messages d'erreur) ;
- contournement du coffre de clés ou de la passphrase ;
- envoi ou suppression de SMS non voulus ;
- injection (fichier CSV, requêtes locales), exécution de code.
