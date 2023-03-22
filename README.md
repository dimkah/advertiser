# Advertiser

**App release notes monitor**

# **How its work?**

Adevertiser is a simple app that running on "DO Function" and runc evry 3 hours. (Other cloud services can be used).

Each run, the app fetch all saved bundle ids from removete database. (Firebase firestore)

Then, the app fetch the app store page of each bundle id and check if there is a current version is the same as the last version in the database.
If there is a new version, the app send a notification to subscribers using Telegram REST API.

# **How to use?**

Service runs automatically every 3 hours.

Too add a new bundle id, you need to add it to the database manualy.

Too add a new subscriber, you need to add it to the database manualy.

# **Technologies stack:**

- Go lang (main language)
- Digital Ocean Function (to run the app)
- Fierbase Firestore (to store data)
- Telegram REST API (to send notifications)

**Requered environment variables:**

TLGRM_BOT_TOKEN - *is a token of your telegram bot*

GCP_CREDS_JSON_BASE64 - *is a base64 encoded json file of your google cloud service account*

**How to deploy?**

For now we are using **"DO Function"** to run the app.

So all deployment operations could be done using "doctl" command line tool. 

Url to documentation: https://docs.digitalocean.com/reference/doctl/how-to/install/

Get all avalible functions: 
```console
doctl serverless functions list
``` 

Deplyment example comand: 
```console
doctl serverless deploy advertiser
```

etc...