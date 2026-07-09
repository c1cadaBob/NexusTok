<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **Passerelle LLM de nouvelle génération et système de gestion des actifs IA**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.en.md">English</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <strong>Français</strong> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="#-démarrage-rapide">Démarrage rapide</a> •
  <a href="#-capacités-clés">Capacités clés</a> •
  <a href="#-déploiement">Déploiement</a> •
  <a href="#-documentation">Documentation</a> •
  <a href="#-licence">Licence</a>
</p>

</div>

## 📝 Description du projet

> [!IMPORTANT]
> - NexusTok est destiné uniquement aux scénarios légalement autorisés de passerelle API IA, d'authentification interne, de routage multi-fournisseurs, d'analyse d'utilisation, de comptabilité des coûts, de facturation et de déploiement privé.
> - Vous devez obtenir légalement les clés API, comptes, services de modèles et autorisations d'interface en amont, et respecter les conditions de service en amont ainsi que les lois applicables.
> - Si vous fournissez des services d'IA générative au public, vous êtes responsable des obligations de conformité exigées par votre juridiction, notamment dépôt, licence, sécurité du contenu, vérification d'identité, conservation des journaux, fiscalité, paiement et autorisations en amont.

NexusTok regroupe les interfaces compatibles OpenAI, Claude, Gemini, Azure, AWS Bedrock et d'autres services IA en amont derrière une API unifiée. Il ajoute des pools de comptes, des contrôles de quota et de facturation, la tarification des modèles, l'observabilité des tâches système, l'analyse de trafic et une console d'administration pour les opérateurs qui souhaitent un plan de contrôle auto-hébergé.

---

## ✨ Capacités clés

| Domaine | Capacité |
|---------|----------|
| Passerelle fournisseurs | Relais unifié pour plus de 40 fournisseurs IA, avec routes compatibles OpenAI, Claude, Gemini, Azure, AWS Bedrock, Responses et tâches asynchrones. |
| Pools de comptes | Pools d'identifiants natifs, groupes de comptes, OAuth/autorisation appareil, contrôles de santé, journaux d'utilisation, audit d'état et liaison aux canaux. |
| Facturation et quotas | Comptabilité token/quota, groupes de prix par modèle, expressions de facturation dynamiques, abonnements, solde portefeuille, codes de rachat et calculs de quota avec saturation. |
| Gouvernance | JWT, OAuth, WebAuthn/Passkeys, catalog d'autorisations admin, Authz au niveau des routes, protection des champs sensibles et audit des opérations d'administration. |
| Exploitation | SystemTask runner, verrous inter-noeuds, heartbeat des instances, nettoyage de journaux, tests de canaux, synchronisation des modèles amont, contrôles des pools de comptes et polling des tâches. |
| Analyse | Tableau de bord, graphiques d'utilisation par modèle/utilisateur, Dashboard Flow en Sankey, journaux d'utilisation, audit de connexion, visibilité sur la saturation de quota et exports. |
| Paiements | Configuration EPay, Stripe, Creem, Waffo et Waffo Pancake, liaison catalog/pair, préparation du checkout hébergé, produits d'abonnement et top-up webhook idempotent. |
| Frontend | Console par défaut React 19, thème classic compatible, vues mobiles des journaux/canaux, rendu riche sécurisé, améliorations Playground et i18n en en/zh/fr/ja/ru/vi. |
| Déploiement | SQLite, MySQL >= 5.7.8, PostgreSQL >= 9.6, cache Redis ou mémoire, Docker Compose, binaire Go, développement hot-reload et notes de packaging Electron. |

---

## 🚀 Démarrage rapide

### Docker Compose (recommandé)

```bash
git clone https://github.com/c1cada/NexusTok.git
cd NexusTok

# Vérifiez docker-compose.yml avant un usage en production.
nano docker-compose.yml

docker-compose up -d
```

Après le démarrage, ouvrez `http://localhost:3000` et terminez l'assistant d'initialisation.

<details>
<summary><strong>Commande Docker</strong></summary>

```bash
docker pull c1cada/nexustok:latest

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cada/nexustok:latest
```

`-v ./data:/data` stocke SQLite et les données d'exécution dans le dossier local `data`. Utilisez un chemin absolu pour les déploiements de production.

</details>

---

## 🚢 Déploiement

| Composant | Exigence |
|-----------|----------|
| Base locale | SQLite avec volume `/data` persistant |
| Base distante | MySQL >= 5.7.8 ou PostgreSQL >= 9.6 |
| Cache | Redis recommandé ; cache mémoire disponible pour les petits déploiements |
| Runtime | Docker / Docker Compose ou binaire Go |

Variables d'environnement fréquentes :

| Variable | Description |
|----------|-------------|
| `SESSION_SECRET` | Requis pour les déploiements multi-instances afin de stabiliser les sessions. |
| `CRYPTO_SECRET` | Requis lorsque Redis ou des données chiffrées partagées sont activés. |
| `SQL_DSN` | Chaîne de connexion MySQL ou PostgreSQL. Vide pour SQLite. |
| `REDIS_CONN_STRING` | Chaîne de connexion Redis. |
| `MAX_REQUEST_BODY_MB` | Limite de taille du corps de requête après décompression. |
| `STREAMING_TIMEOUT` | Délai d'expiration des flux en secondes. |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Tampon maximal pour les grands fragments de flux. |

Consultez [Déploiement et maintenance](./docs/installation/deployment.md) pour la production, le développement hot-reload, le déploiement binaire, le changement de thème et le dépannage.

---

## 📚 Documentation

| Sujet | Lien |
|-------|------|
| Déploiement et maintenance | [docs/installation/deployment.md](./docs/installation/deployment.md) |
| Installation BT/Baota | [docs/installation/BT.md](./docs/installation/BT.md) |
| Paramètres avancés des canaux | [docs/configuration/channel-other-settings.md](./docs/configuration/channel-other-settings.md) |
| OpenAPI backend | [docs/openapi/api.json](./docs/openapi/api.json) |
| OpenAPI relay | [docs/openapi/relay.json](./docs/openapi/relay.json) |
| Application Electron | [electron/README.md](./electron/README.md) |
| Issues | [GitHub Issues](https://github.com/c1cada/NexusTok/issues) |
| Versions | [GitHub Releases](https://github.com/c1cada/NexusTok/releases) |

---

## 📜 Licence

NexusTok est distribué sous la [GNU Affero General Public License v3.0](./LICENSE).

Ce projet est basé sur [One API](https://github.com/songquanpeng/one-api), sous licence MIT.

Si la politique de votre organisation ne permet pas l'utilisation de logiciels AGPLv3, ou si vous avez besoin d'une licence commerciale, contactez [support@c1cada.dev](mailto:support@c1cada.dev).

---

<div align="center">

### Merci d'utiliser NexusTok

**[Dépôt](https://github.com/c1cada/NexusTok)** • **[Issues](https://github.com/c1cada/NexusTok/issues)** • **[Versions](https://github.com/c1cada/NexusTok/releases)**

<sub>Construit par c1cada et les contributeurs.</sub>

</div>
