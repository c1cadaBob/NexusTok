<div align="center">

![NexusTok](/web/public/logo.png)

# NexusTok

🍥 **Passerelle de modèles étendus de nouvelle génération et système de gestion d'actifs d'IA**

<p align="center">
  <a href="./README.zh_CN.md">简体中文</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <strong>Français</strong> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/c1cadabob/nexustok/main/LICENSE">
    <img src="https://img.shields.io/github/license/c1cadabob/nexustok?color=brightgreen" alt="licence">
  </a><!--
  --><a href="https://github.com/c1cadabob/nexustok/releases/latest">
    <img src="https://img.shields.io/github/v/release/c1cadabob/nexustok?color=brightgreen&include_prereleases" alt="version">
  </a><!--
  --><a href="https://hub.docker.com/r/c1cadabob/nexustok">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-démarrage-rapide">Démarrage rapide</a> •
  <a href="#-fonctionnalités-clés">Fonctionnalités clés</a> •
  <a href="#-déploiement">Déploiement</a>
</p>

</div>

## 📝 Description du projet

> [!IMPORTANT]
> - Ce projet est exclusivement destiné aux scénarios de passerelle API d'IA légalement autorisés, d'authentification organisationnelle, de gestion multi-modèles, d'analyse d'utilisation, de comptabilisation des coûts et de déploiement privé.
> - Les utilisateurs doivent obtenir légalement les clés API, comptes, services de modèles et autorisations d'interface en amont, et doivent respecter les conditions d'utilisation en amont et les lois et réglementations applicables.
> - Les utilisateurs doivent s'assurer que leur utilisation est conforme aux conditions d'utilisation en amont et aux lois et réglementations applicables.
> - Lors de la fourniture de services d'IA générative au public, les utilisateurs doivent se conformer aux exigences réglementaires applicables et remplir toutes les obligations d'enregistrement, de licence, de sécurité du contenu, de vérification d'identité, de conservation des journaux, de fiscalité et d'autorisation en amont requises par leur juridiction.

## 🚀 Démarrage rapide

### Utilisation de Docker Compose (recommandé)

```bash
# Cloner le projet et lancer le déploiement de production
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

<details>
<summary><strong>Utilisation des commandes Docker</strong></summary>

```bash
# Mode de compatibilité mono-conteneur (SQLite, sans PostgreSQL ni Redis)
docker run --name NexusTok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v ./data:/data \
  -v ./logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

> Cette commande ne démarre pas PostgreSQL ni Redis. Utilisez Compose pour la topologie de
> production complète. SQLite n'est pas migré automatiquement vers PostgreSQL.

</details>

---

🎉 Après le déploiement, visitez `http://localhost:3030` pour commencer à utiliser!

> [!WARNING]
> Lorsque vous exploitez ce projet en tant que service public d'IA générative ou service de revente d'API, les utilisateurs doivent d'abord remplir toutes les obligations requises en matière d'enregistrement, de licence, de sécurité du contenu, de vérification d'identité, de conservation des journaux, de fiscalité, de paiement et d'autorisation en amont.

📖 Pour plus de méthodes de déploiement, veuillez vous référer à [Guide de déploiement](https://docs.nexustok.ai/en/docs/installation)

## ✨ Fonctionnalités clés

> Pour les fonctionnalités détaillées, veuillez vous référer à [Présentation des fonctionnalités](https://docs.nexustok.ai/en/docs/guide/wiki/basic-concepts/features-introduction) |

### 🎨 Fonctions principales

| Fonctionnalité | Description |
|------|------|
| 🎨 Nouvelle interface utilisateur | Conception d'interface utilisateur moderne |
| 🌍 Multilingue | Prend en charge le chinois simplifié, le chinois traditionnel, l'anglais, le français et le japonais |
| 🔄 Compatibilité des données | Complètement compatible avec la base de données originale de One API |
| 📈 Tableau de bord des données | Console visuelle et analyse statistique |
| 🔒 Gestion des permissions | Regroupement de jetons, restrictions de modèles, gestion des utilisateurs |

### 💰 Comptabilisation et facturation des usages autorisés

- ✅ Rechargement interne et allocation de quotas pour les scénarios légalement autorisés (EPay, Stripe)
- ✅ Comptabilisation des coûts par requête, par utilisation et par hit de cache au niveau organisationnel
- ✅ Statistiques de facturation du cache pour OpenAI, Azure, DeepSeek, Claude, Qwen et les modèles pris en charge
- ✅ Politiques de facturation flexibles pour la gestion interne ou les clients entreprise autorisés

### 🔐 Autorisation et sécurité

- 😈 Connexion par autorisation Discord
- 🤖 Connexion par autorisation LinuxDO
- 📱 Connexion par autorisation Telegram
- 🔑 Authentification unifiée OIDC
- 🔍 Requête de quota d'utilisation de clé (avec [NexusTok-key-tool](https://github.com/c1cadabob/nexustok-key-tool))

### 🚀 Fonctionnalités avancées

**Prise en charge des formats d'API:**
- ⚡ [OpenAI Responses](https://docs.nexustok.ai/en/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.nexustok.ai/en/docs/api/ai-model/realtime/create-realtime-session) (y compris Azure)
- ⚡ [Claude Messages](https://docs.nexustok.ai/en/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://docs.nexustok.ai/en/api/google-gemini-chat)
- 🔄 [Modèles Rerank](https://docs.nexustok.ai/en/docs/api/ai-model/rerank/create-rerank) (Cohere, Jina)

**Routage intelligent:**
- ⚖️ Sélection aléatoire pondérée des canaux
- 🔄 Nouvelle tentative automatique en cas d'échec
- 🚦 Limitation du débit du modèle pour les utilisateurs

**Conversion de format:**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - Texte uniquement, les appels de fonction ne sont pas encore pris en charge
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - En développement
- 🔄 **Fonctionnalité de la pensée au contenu**

**Prise en charge de l'effort de raisonnement:**

<details>
<summary>Voir la configuration détaillée</summary>

**Modèles de la série OpenAI :**
- `o3-mini-high` - Effort de raisonnement élevé
- `o3-mini-medium` - Effort de raisonnement moyen
- `o3-mini-low` - Effort de raisonnement faible
- `gpt-5-high` - Effort de raisonnement élevé
- `gpt-5-medium` - Effort de raisonnement moyen
- `gpt-5-low` - Effort de raisonnement faible

**Modèles de pensée de Claude:**
- `claude-3-7-sonnet-20250219-thinking` - Activer le mode de pensée

**Modèles de la série Google Gemini:**
- `gemini-2.5-flash-thinking` - Activer le mode de pensée
- `gemini-2.5-flash-nothinking` - Désactiver le mode de pensée
- `gemini-2.5-pro-thinking` - Activer le mode de pensée
- `gemini-2.5-pro-thinking-128` - Activer le mode de pensée avec budget de pensée de 128 tokens
- Vous pouvez également ajouter les suffixes `-low`, `-medium` ou `-high` aux modèles Gemini pour fixer le niveau d’effort de raisonnement (sans suffixe de budget supplémentaire).

</details>

## 🚢 Déploiement

> [!TIP]
> **Dernière image Docker:** `c1cadabob/nexustok:latest`
>
> **Image v0.2.3:** `c1cadabob/nexustok:v0.2.3`

### 📋 Exigences de déploiement

| Composant | Exigence |
|------|------|
| **Base de données de production** | PostgreSQL 15 (l'application prend aussi en charge PostgreSQL ≥ 9.6) |
| **Cache de production** | Redis 7 sur le réseau interne Compose |
| **Bases compatibles** | MySQL ≥ 5.7.8 et SQLite |
| **Moteur de conteneur** | Docker / Docker Compose |
| **Architecture système** | 64 bits uniquement (amd64 / arm64) ; les systèmes 32 bits ne sont pas pris en charge |

> Compose est le point d'entrée recommandé pour la production. La commande mono-conteneur
> est uniquement un mode de compatibilité SQLite/sans Redis ; PostgreSQL et Redis ne sont pas
> créés automatiquement et un fichier SQLite existant n'est jamais migré automatiquement.

### ⚙️ Configuration des variables d'environnement

<details>
<summary>Configuration courante des variables d'environnement</summary>

| Nom de variable | Description | Valeur par défaut |
|--------|------|--------|
| `SESSION_SECRET` | Secret de signature d’authentification, identique sur tous les nœuds | - |
| `SESSION_COOKIE_SECURE` | `false`/non défini désactive l’OriginGuard de refresh/logout pour les proxys HTTP locaux ; `true` active le cookie Secure et le contrôle strict de l’Origin | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Obligatoire en mode Secure : Origins HTTPS exactes autorisées pour refresh/logout, séparées par des virgules ; ce n’est pas une liste CORS relay | - |
| `TRUSTED_PROXIES` | Variable absente/vide : approuve le bouclage, les réseaux RFC 1918 et l’ULA IPv6 avec un avertissement au démarrage ; `none` n’approuve aucun proxy ; une liste IP/CIDR explicite remplace les valeurs par défaut | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | Nombre maximal de Sessions de connexion actives par utilisateur | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | Nombre maximal de Sessions créées par utilisateur dans la fenêtre, y compris les Sessions révoquées | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Fenêtre de comptage des Sessions ; limitée à la durée de conservation des Sessions révoquées si elle est supérieure | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | Conservation en jours des Sessions révoquées pour l’audit et le comptage | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | Seuil global horaire déclenchant uniquement une alerte, sans bloquer les connexions | `5000` |
| `CRYPTO_SECRET` | Secret HMAC des clés de cache ; les nœuds partageant Redis doivent utiliser la même valeur effective | Par défaut, `SESSION_SECRET` |
| `SQL_DSN` | Chaine de connexion à la base de données | - |
| `REDIS_CONN_STRING` | Chaine de connexion Redis | - |
| `STREAMING_TIMEOUT` | Délai d'expiration du streaming (secondes) | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | Taille max du buffer par ligne (Mo) pour le scanner SSE ; à augmenter quand les sorties image/base64 sont très volumineuses (ex. images 4K) | `64` |
| `MAX_REQUEST_BODY_MB` | Taille maximale du corps de requête (Mo, comptée **après décompression** ; évite les requêtes énormes/zip bombs qui saturent la mémoire). Dépassement ⇒ `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Version de l'API Azure | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | Interrupteur du journal d'erreurs | `false` |
| `PYROSCOPE_URL` | Adresse du serveur Pyroscope | - |
| `PYROSCOPE_APP_NAME` | Nom de l'application Pyroscope | `NexusTok` |
| `PYROSCOPE_BASIC_AUTH_USER` | Utilisateur Basic Auth Pyroscope | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Mot de passe Basic Auth Pyroscope | - |
| `PYROSCOPE_MUTEX_RATE` | Taux d'échantillonnage mutex Pyroscope | `5` |
| `PYROSCOPE_BLOCK_RATE` | Taux d'échantillonnage block Pyroscope | `5` |
| `HOSTNAME` | Nom d'hôte tagué pour Pyroscope | `NexusTok` |

📖 **Configuration complète:** [Documentation des variables d'environnement](https://docs.nexustok.ai/en/docs/installation/config-maintenance/environment-variables)

</details>

### 🖥️ Déploiement sur une machine (PostgreSQL + Redis)

Préparez un hôte Linux 64 bits avec Docker Engine, Docker Compose v2, Git et `curl`.
N'ouvrez que le port `3030` dans le pare-feu, ou seulement `80/443` si un proxy inverse HTTPS
est l'entrée publique. PostgreSQL `5432` et Redis `6379` restent sur le réseau interne Compose
et ne doivent pas être exposés sur Internet.

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

Au premier lancement, le script crée un `.env` en mode `0600` et génère
`POSTGRES_PASSWORD` et `REDIS_PASSWORD`. Les mots de passe existants sont conservés. Le script
valide `docker compose config`, télécharge l'image, démarre PostgreSQL, Redis et NexusTok, puis
attend les trois contrôles de santé.

```bash
docker compose ps
curl http://127.0.0.1:3030/api/status
curl http://127.0.0.1:3030/api/setup
```

Après une réponse réussie, ouvrez `http://adresse-du-serveur:3030`, terminez l'assistant de
configuration et créez le compte administrateur. Le proxy inverse doit transmettre SSE,
WebSocket, les connexions longues et les en-têtes `X-Forwarded-*`. Avec HTTPS, activez
`SESSION_COOKIE_SECURE=true` et configurez `SESSION_COOKIE_TRUSTED_URL` avec l'Origin HTTPS
exacte. `TRUSTED_PROXIES` doit contenir uniquement les IP/CIDR des proxies de confiance.

La commande mono-conteneur suivante est uniquement un mode de compatibilité :

```bash
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.3
```

Elle ne démarre ni PostgreSQL ni Redis. Avant une mise à niveau, sauvegardez `.env`,
`/opt/nexustok/data`, `/opt/nexustok/logs` et le volume nommé PostgreSQL :

```bash
docker compose exec -T postgres pg_dump -U root nexustok > nexustok-$(date +%F).sql
cp .env /secure-backup/nexustok.env
docker compose pull
docker compose up -d
```

Pour revenir en arrière, épinglez l'image à la version vérifiée précédente, puis exécutez
`docker compose up -d`. Ne supprimez pas le volume
PostgreSQL. SQLite n'est pas migré automatiquement vers PostgreSQL : les données existantes
doivent être sauvegardées et migrées séparément avec une procédure vérifiée.

Problèmes fréquents : port `3030` occupé, PostgreSQL/Redis non sains
(`docker compose logs postgres redis`), mot de passe `.env` vide ou modifié, permissions ou
SELinux/AppArmor, image amd64/arm64 incompatible, délais du proxy inverse ou en-têtes Upgrade
manquants pour SSE/WebSocket. Le socket Docker donne un contrôle du moteur de l'hôte et ne doit
être monté que sur une machine administrée de confiance.

Référence de dépannage : [FAQ](https://docs.nexustok.ai/en/docs/support/faq).

### 🌐 Déploiement multi-machines

Ne démarrez pas PostgreSQL et Redis intégrés sur chaque nœud applicatif. Préparez un PostgreSQL
partagé et un Redis partagé, protégés par le réseau privé, les contrôles d'accès et TLS. Tous les
nœuds utilisent les mêmes `SQL_DSN`, `REDIS_CONN_STRING`, `SESSION_SECRET` et `CRYPTO_SECRET`,
mais chaque `NODE_NAME` doit être unique.

Le nœud principal laisse `NODE_TYPE` non défini et exécute les migrations et les tâches système.
Les nœuds secondaires définissent `NODE_TYPE=slave` et servent les requêtes. Synchronisez les
horloges, utilisez un équilibreur avec `/api/status` comme contrôle de santé et retirez un nœud
non sain avant toute intervention.

```bash
docker run --name nexustok-node-1 -d --restart always \
  -p 3030:3030 \
  --env-file /etc/nexustok/node.env \
  -e NODE_NAME=node-1 \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.3
```

`node.env` doit au minimum contenir `SQL_DSN`, `REDIS_CONN_STRING`, `SESSION_SECRET` et
`CRYPTO_SECRET`; ajoutez `NODE_TYPE=slave` sur les nœuds secondaires. Après les migrations du
principal, mettez à niveau les secondaires un par un, vérifiez-les, puis réintégrez-les à
l'équilibreur avant de traiter le principal pendant la fenêtre de maintenance. Préparez une
sauvegarde PostgreSQL testée avant tout retour arrière.

Un Redis partagé permet de partager Sessions, limitations et cache de contrôle. Des Redis
indépendants produisent une propagation différée et des limites locales. Sans Redis, les Sessions
reviennent à la base et les limites restent dans chaque processus. Le socket Docker, les journaux
locaux et `/data` ne constituent pas un stockage partagé entre nœuds ; les journaux centralisés
et les fichiers partagés nécessitent une conception séparée.

### 🔄 Nouvelle tentative de canal et cache

**Configuration de la nouvelle tentative:** `Paramètres → Paramètres de fonctionnement → Paramètres généraux → Nombre de tentatives en cas d'échec`

**Configuration du cache:**
- `REDIS_CONN_STRING`: Cache Redis (recommandé)
- `MEMORY_CACHE_ENABLED`: Cache mémoire

## 📜 Licence

Ce projet est sous licence [GNU Affero General Public License v3.0 (AGPLv3)](./LICENSE).

Il s'agit d'un projet open-source développé sur la base de [One API](https://github.com/songquanpeng/one-api) (licence MIT).

Si les politiques de votre organisation ne permettent pas l'utilisation de logiciels sous licence AGPLv3, ou si vous souhaitez éviter les obligations open-source de l'AGPLv3, veuillez nous contacter à : [support@c1cadabob.dev](mailto:support@c1cadabob.dev)

<div align="center">

### 💖 Merci d'utiliser NexusTok

Si ce projet vous est utile, bienvenue à nous donner une ⭐️ Étoile！

**[Commentaires sur les problèmes](https://github.com/c1cadabob/nexustok/issues)** • **[Dernière version](https://github.com/c1cadabob/nexustok/releases)**

<sub>Construit avec ❤️ par c1cadaBob</sub>

</div>
