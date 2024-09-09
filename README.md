# Advertiser

**App release notes monitor**

# **How its work?**

**Function** - monitor/start

Is a simple app that running on "DO Function" and runc evry 3 hours. (Other cloud services can be used).

The app fetch all saved bundle ids from removete database. (Firebase firestore)

Then, the app fetch the app store page of each bundle id and check if there is a current version is the same as the last version in the database.
If there is a new version, the app send a notification to subscribers using Telegram REST API.

**Function** - monitor/subscriber

Is a simple implementation of telegram webhook listener.
Used to add or remove subscribers from the database.

# **How to use?**

Service runs automatically every 3 hours.

Too add a new bundle id, you need to add it to the database manualy.

Too manage subsctibers, you need to add telegram bot **@cropwiseappsbot** to your chat and send ``` /subscribe ``` command, to subscribe to the notifications.

Send ``` /unsubscribe ``` command, to unsubscribe from the notifications.

# **Technologies stack:**

- Go lang (main language)
- Digital Ocean Function (to run the app)
- Fierbase Firestore (to store data)
- Telegram REST API (to send notifications)

**Requered environment variables:**

TLGRM_BOT_TOKEN - *is a token of your telegram bot* *(Coould 
be obtained from BotFather)*
GCP_CREDS_JSON_BASE64 - *is a base64 encoded json file of your google cloud service account* *(Could be obtained from Fierbase console)*

*(pay attention, we have to **set environment variables each time after deploy**, since DO reset it after each deploy)*

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