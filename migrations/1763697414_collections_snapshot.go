package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		jsonData := `[
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text455797646",
						"max": 0,
						"min": 0,
						"name": "collectionRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text127846527",
						"max": 0,
						"min": 0,
						"name": "recordRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text1582905952",
						"max": 0,
						"min": 0,
						"name": "method",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": true,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": true,
						"type": "autodate"
					}
				],
				"id": "pbc_2279338944",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_mfas_collectionRef_recordRef` + "`" + ` ON ` + "`" + `_mfas` + "`" + ` (collectionRef,recordRef)"
				],
				"listRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId",
				"name": "_mfas",
				"system": true,
				"type": "base",
				"updateRule": null,
				"viewRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId"
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text455797646",
						"max": 0,
						"min": 0,
						"name": "collectionRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text127846527",
						"max": 0,
						"min": 0,
						"name": "recordRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cost": 8,
						"hidden": true,
						"id": "password901924565",
						"max": 0,
						"min": 0,
						"name": "password",
						"pattern": "",
						"presentable": false,
						"required": true,
						"system": true,
						"type": "password"
					},
					{
						"autogeneratePattern": "",
						"hidden": true,
						"id": "text3866985172",
						"max": 0,
						"min": 0,
						"name": "sentTo",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": true,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": true,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": true,
						"type": "autodate"
					}
				],
				"id": "pbc_1638494021",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_otps_collectionRef_recordRef` + "`" + ` ON ` + "`" + `_otps` + "`" + ` (collectionRef, recordRef)"
				],
				"listRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId",
				"name": "_otps",
				"system": true,
				"type": "base",
				"updateRule": null,
				"viewRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId"
			},
			{
				"createRule": null,
				"deleteRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId",
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text455797646",
						"max": 0,
						"min": 0,
						"name": "collectionRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text127846527",
						"max": 0,
						"min": 0,
						"name": "recordRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text2462348188",
						"max": 0,
						"min": 0,
						"name": "provider",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text1044722854",
						"max": 0,
						"min": 0,
						"name": "providerId",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": true,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": true,
						"type": "autodate"
					}
				],
				"id": "pbc_2281828961",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_externalAuths_record_provider` + "`" + ` ON ` + "`" + `_externalAuths` + "`" + ` (collectionRef, recordRef, provider)",
					"CREATE UNIQUE INDEX ` + "`" + `idx_externalAuths_collection_provider` + "`" + ` ON ` + "`" + `_externalAuths` + "`" + ` (collectionRef, provider, providerId)"
				],
				"listRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId",
				"name": "_externalAuths",
				"system": true,
				"type": "base",
				"updateRule": null,
				"viewRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId"
			},
			{
				"createRule": null,
				"deleteRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId",
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text455797646",
						"max": 0,
						"min": 0,
						"name": "collectionRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text127846527",
						"max": 0,
						"min": 0,
						"name": "recordRef",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text4228609354",
						"max": 0,
						"min": 0,
						"name": "fingerprint",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": true,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": true,
						"type": "autodate"
					}
				],
				"id": "pbc_4275539003",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_authOrigins_unique_pairs` + "`" + ` ON ` + "`" + `_authOrigins` + "`" + ` (collectionRef, recordRef, fingerprint)"
				],
				"listRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId",
				"name": "_authOrigins",
				"system": true,
				"type": "base",
				"updateRule": null,
				"viewRule": "@request.auth.id != '' && recordRef = @request.auth.id && collectionRef = @request.auth.collectionId"
			},
			{
				"authAlert": {
					"emailTemplate": {
						"body": "<p>Hello,</p>\n<p>We noticed a login to your {APP_NAME} account from a new location:</p>\n<p><em>{ALERT_INFO}</em></p>\n<p><strong>If this wasn't you, you should immediately change your {APP_NAME} account password to revoke access from all other locations.</strong></p>\n<p>If this was you, you may disregard this email.</p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
						"subject": "Login from a new location"
					},
					"enabled": true
				},
				"authRule": "",
				"authToken": {
					"duration": 1209600
				},
				"confirmEmailChangeTemplate": {
					"body": "<p>Hello,</p>\n<p>Click on the button below to confirm your new email address.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-email-change/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Confirm new email</a>\n</p>\n<p><i>If you didn't ask to change your email address, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Confirm your {APP_NAME} new email address"
				},
				"createRule": null,
				"deleteRule": null,
				"emailChangeToken": {
					"duration": 1800
				},
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cost": 0,
						"hidden": true,
						"id": "password901924565",
						"max": 0,
						"min": 8,
						"name": "password",
						"pattern": "",
						"presentable": false,
						"required": true,
						"system": true,
						"type": "password"
					},
					{
						"autogeneratePattern": "[a-zA-Z0-9]{50}",
						"hidden": true,
						"id": "text2504183744",
						"max": 60,
						"min": 30,
						"name": "tokenKey",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"exceptDomains": null,
						"hidden": false,
						"id": "email3885137012",
						"name": "email",
						"onlyDomains": null,
						"presentable": false,
						"required": true,
						"system": true,
						"type": "email"
					},
					{
						"hidden": false,
						"id": "bool1547992806",
						"name": "emailVisibility",
						"presentable": false,
						"required": false,
						"system": true,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "bool256245529",
						"name": "verified",
						"presentable": false,
						"required": false,
						"system": true,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": true,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": true,
						"type": "autodate"
					}
				],
				"fileToken": {
					"duration": 120
				},
				"id": "pbc_3142635823",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `idx_tokenKey_pbc_3142635823` + "`" + ` ON ` + "`" + `_superusers` + "`" + ` (` + "`" + `tokenKey` + "`" + `)",
					"CREATE UNIQUE INDEX ` + "`" + `idx_email_pbc_3142635823` + "`" + ` ON ` + "`" + `_superusers` + "`" + ` (` + "`" + `email` + "`" + `) WHERE ` + "`" + `email` + "`" + ` != ''"
				],
				"listRule": null,
				"manageRule": null,
				"mfa": {
					"duration": 1800,
					"enabled": true,
					"rule": ""
				},
				"name": "_superusers",
				"oauth2": {
					"enabled": false,
					"mappedFields": {
						"avatarURL": "",
						"id": "",
						"name": "",
						"username": ""
					}
				},
				"otp": {
					"duration": 180,
					"emailTemplate": {
						"body": "<p>Hello,</p>\n<p>Your one-time password is: <strong>{OTP}</strong></p>\n<p><i>If you didn't ask for the one-time password, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
						"subject": "OTP for {APP_NAME}"
					},
					"enabled": true,
					"length": 4
				},
				"passwordAuth": {
					"enabled": true,
					"identityFields": [
						"email"
					]
				},
				"passwordResetToken": {
					"duration": 1800
				},
				"resetPasswordTemplate": {
					"body": "<p>Hello,</p>\n<p>Click on the button below to reset your password.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-password-reset/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Reset password</a>\n</p>\n<p><i>If you didn't ask to reset your password, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Reset your {APP_NAME} password"
				},
				"system": true,
				"type": "auth",
				"updateRule": null,
				"verificationTemplate": {
					"body": "<p>Hello,</p>\n<p>Thank you for joining us at {APP_NAME}.</p>\n<p>Click on the button below to verify your email address.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-verification/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Verify</a>\n</p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Verify your {APP_NAME} email"
				},
				"verificationToken": {
					"duration": 259200
				},
				"viewRule": null
			},
			{
				"authAlert": {
					"emailTemplate": {
						"body": "<p>Hello,</p>\n<p>We noticed a login to your {APP_NAME} account from a new location:</p>\n<p><em>{ALERT_INFO}</em></p>\n<p><strong>If this wasn't you, you should immediately change your {APP_NAME} account password to revoke access from all other locations.</strong></p>\n<p>If this was you, you may disregard this email.</p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
						"subject": "Login from a new location"
					},
					"enabled": true
				},
				"authRule": "",
				"authToken": {
					"duration": 1209600
				},
				"confirmEmailChangeTemplate": {
					"body": "<p>Hello,</p>\n<p>Click on the button below to confirm your new email address.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/_/#/auth/confirm-email-change/{TOKEN}\" target=\"_blank\" rel=\"noopener\">Confirm new email</a>\n</p>\n<p><i>If you didn't ask to change your email address, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Confirm your {APP_NAME} new email address"
				},
				"createRule": "@request.context = 'oauth2' || (@request.body.email:isset = true && @request.body.name:isset = true && @request.body.password:isset = true && @request.body.email:isset = true && @request.body.emailVisibility = true)",
				"deleteRule": null,
				"emailChangeToken": {
					"duration": 1800
				},
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cost": 10,
						"hidden": true,
						"id": "password901924565",
						"max": 0,
						"min": 6,
						"name": "password",
						"pattern": "",
						"presentable": false,
						"required": true,
						"system": true,
						"type": "password"
					},
					{
						"autogeneratePattern": "[a-zA-Z0-9_]{50}",
						"hidden": true,
						"id": "text2504183744",
						"max": 60,
						"min": 30,
						"name": "tokenKey",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"exceptDomains": null,
						"hidden": false,
						"id": "email3885137012",
						"name": "email",
						"onlyDomains": null,
						"presentable": false,
						"required": false,
						"system": true,
						"type": "email"
					},
					{
						"hidden": false,
						"id": "bool1547992806",
						"name": "emailVisibility",
						"presentable": false,
						"required": false,
						"system": true,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "bool256245529",
						"name": "verified",
						"presentable": false,
						"required": false,
						"system": true,
						"type": "bool"
					},
					{
						"autogeneratePattern": "user[0-9]{5}[A-Za-z]",
						"hidden": false,
						"id": "s4xgbyzv",
						"max": 50,
						"min": 2,
						"name": "name",
						"pattern": "^[A-Za-z][\\w\\s\\.\\-']*$",
						"presentable": false,
						"primaryKey": false,
						"required": true,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "deehajec",
						"name": "disabled",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "date3359898891",
						"max": "",
						"min": "",
						"name": "last_login",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "date"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"fileToken": {
					"duration": 120
				},
				"id": "flm2xtzrt82ltf7",
				"indexes": [
					"CREATE UNIQUE INDEX ` + "`" + `_flm2xtzrt82ltf7_email_idx` + "`" + ` ON ` + "`" + `users` + "`" + ` (` + "`" + `email` + "`" + `) WHERE ` + "`" + `email` + "`" + ` != ''",
					"CREATE UNIQUE INDEX ` + "`" + `_flm2xtzrt82ltf7_tokenKey_idx` + "`" + ` ON ` + "`" + `users` + "`" + ` (` + "`" + `tokenKey` + "`" + `)"
				],
				"listRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.role ?= 'administrator'",
				"manageRule": null,
				"mfa": {
					"duration": 1800,
					"enabled": true,
					"rule": "@request.context != 'oauth2'"
				},
				"name": "users",
				"oauth2": {
					"enabled": true,
					"mappedFields": {
						"avatarURL": "",
						"id": "",
						"name": "name",
						"username": ""
					}
				},
				"otp": {
					"duration": 180,
					"emailTemplate": {
						"body": "<p>Hello,</p>\n<p>Your one-time password is: <strong>{OTP}</strong></p>\n<p><i>If you didn't ask for the one-time password, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
						"subject": "OTP for {APP_NAME}"
					},
					"enabled": true,
					"length": 4
				},
				"passwordAuth": {
					"enabled": true,
					"identityFields": [
						"email"
					]
				},
				"passwordResetToken": {
					"duration": 1800
				},
				"resetPasswordTemplate": {
					"body": "<p>Hello,</p>\n<p>Click on the button below to reset your password.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/usermgmt?mode=resetPassword&oobCode={TOKEN}\" target=\"_blank\" rel=\"noopener\">Reset password</a>\n</p>\n<p><i>If you didn't ask to reset your password, you can ignore this email.</i></p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Reset your {APP_NAME} password"
				},
				"system": false,
				"type": "auth",
				"updateRule": "@request.auth.id != \"\" && @request.auth.id = id",
				"verificationTemplate": {
					"body": "<p>Hello,</p>\n<p>Thank you for joining us at {APP_NAME}.</p>\n<p>Click on the button below to verify your email address.</p>\n<p>\n  <a class=\"btn\" href=\"{APP_URL}/usermgmt?mode=verifyEmail&oobCode={TOKEN}\" target=\"_blank\" rel=\"noopener\">Verify</a>\n</p>\n<p>\n  Thanks,<br/>\n  {APP_NAME} team\n</p>",
					"subject": "Verify your {APP_NAME} email"
				},
				"verificationToken": {
					"duration": 604800
				},
				"viewRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.role ?= 'administrator'"
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "giek43fp",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": true,
						"collectionId": "kyfdlowtckhj9wm",
						"hidden": false,
						"id": "g01gueai",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "territory",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": true,
						"collectionId": "rupq6yj561mghrr",
						"hidden": false,
						"id": "6bpfvzkp",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "map",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"hidden": false,
						"id": "sjdy6xys",
						"max": null,
						"min": null,
						"name": "floor",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "hptttvad",
						"max": 0,
						"min": 0,
						"name": "code",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"cascadeDelete": false,
						"collectionId": "wz7avhl19otivv6",
						"hidden": false,
						"id": "avthtafh",
						"maxSelect": 2147483647,
						"minSelect": 0,
						"name": "type",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"hidden": false,
						"id": "agrc9m5o",
						"maxSelect": 1,
						"name": "status",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"not_done",
							"done",
							"not_home",
							"do_not_call",
							"invalid"
						]
					},
					{
						"hidden": false,
						"id": "p5927pws",
						"max": null,
						"min": null,
						"name": "sequence",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "d33qfsys",
						"max": null,
						"min": null,
						"name": "not_home_tries",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "cygablhk",
						"max": 0,
						"min": 0,
						"name": "notes",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "wfd4nxfq",
						"max": "",
						"min": "",
						"name": "last_notes_updated",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "date"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "phiszer0",
						"max": 0,
						"min": 0,
						"name": "last_notes_updated_by",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "ujtyptly",
						"max": "",
						"min": "",
						"name": "dnc_time",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "date"
					},
					{
						"hidden": false,
						"id": "ri9uyzlz",
						"maxSize": 2000000,
						"name": "coordinates",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "json"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "text385774305",
						"max": 0,
						"min": 0,
						"name": "updated_by",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "thnq0jvp13lr8ct",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_7CBdHug` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (` + "`" + `map` + "`" + `)",
					"CREATE INDEX ` + "`" + `idx_vRAy883` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (\n  ` + "`" + `code` + "`" + `,\n  ` + "`" + `map` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_20F0iUx` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (\n  ` + "`" + `floor` + "`" + `,\n  ` + "`" + `map` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_Fx581hd` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (\n  ` + "`" + `map` + "`" + `,\n  ` + "`" + `status` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_vEPqxAk` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (\n  ` + "`" + `territory` + "`" + `,\n  ` + "`" + `status` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_124WdYx7Ri` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `congregation` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_kJtt908M2s` + "`" + ` ON ` + "`" + `addresses` + "`" + ` (\n  ` + "`" + `congregation` + "`" + `,\n  ` + "`" + `last_notes_updated_by` + "`" + `\n)"
				],
				"listRule": "// Reduce role joins for registered users as adds are huge\n@request.auth.id != \"\" || (@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map && @collection.assignments:link.expiry_date > @now)",
				"name": "addresses",
				"system": false,
				"type": "base",
				"updateRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map && @collection.assignments:link.expiry_date > @now)",
				"viewRule": null
			},
			{
				"createRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && (@collection.roles:access.role ?= 'administrator' || @collection.roles:access.role ?= 'conductor')",
				"deleteRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && (@collection.roles:access.role ?= 'administrator' || @collection.roles:access.role ?= 'conductor')",
				"fields": [
					{
						"autogeneratePattern": "[a-zA-Z0-9]{20}",
						"hidden": false,
						"id": "text3208210256",
						"max": 20,
						"min": 20,
						"name": "id",
						"pattern": "^[a-zA-Z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "relation2104863268",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": true,
						"collectionId": "rupq6yj561mghrr",
						"hidden": false,
						"id": "kdnmbi1t",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "map",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": false,
						"collectionId": "flm2xtzrt82ltf7",
						"hidden": false,
						"id": "56zovha9",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "user",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"hidden": false,
						"id": "6uegqyqw",
						"maxSelect": 1,
						"name": "type",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"normal",
							"personal"
						]
					},
					{
						"hidden": false,
						"id": "rcjynvmi",
						"max": "",
						"min": "",
						"name": "expiry_date",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "date"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "afm4d6iw",
						"max": 0,
						"min": 0,
						"name": "publisher",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "06zc4itse2ipw9l",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_pI4sxv2` + "`" + ` ON ` + "`" + `assignments` + "`" + ` (` + "`" + `map` + "`" + `)",
					"CREATE INDEX ` + "`" + `idx_RuF9QNcKE2` + "`" + ` ON ` + "`" + `assignments` + "`" + ` (` + "`" + `expiry_date` + "`" + `)",
					"CREATE INDEX ` + "`" + `idx_6V9YIvnGqD` + "`" + ` ON ` + "`" + `assignments` + "`" + ` (\n  ` + "`" + `user` + "`" + `,\n  ` + "`" + `created` + "`" + `\n)"
				],
				"listRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && expiry_date > @now",
				"name": "assignments",
				"system": false,
				"type": "base",
				"updateRule": null,
				"viewRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id = id && expiry_date > @now)"
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "esjglexz",
						"max": 0,
						"min": 0,
						"name": "code",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "naozar5p",
						"max": 0,
						"min": 0,
						"name": "name",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "jamr0gou",
						"max": null,
						"min": null,
						"name": "expiry_hours",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "yb3nx2pi",
						"max": null,
						"min": null,
						"name": "max_tries",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "ggikdoy5",
						"maxSelect": 1,
						"name": "origin",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"sg",
							"my"
						]
					},
					{
						"hidden": false,
						"id": "6eofw7xa",
						"maxSelect": 1,
						"name": "timezone",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"Asia/Singapore",
							"Asia/Kuala_Lumpur"
						]
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "zzljam3htisq5tv",
				"indexes": [],
				"listRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= id",
				"name": "congregations",
				"system": false,
				"type": "base",
				"updateRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= id && @collection.roles:access.role ?= 'administrator'",
				"viewRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= id) || (@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.congregation ?= id && @collection.assignments:link.expiry_date > @now)"
			},
			{
				"createRule": null,
				"deleteRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "ttfueaey",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": true,
						"collectionId": "kyfdlowtckhj9wm",
						"hidden": false,
						"id": "xdh2kztc",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "territory",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "kzw1eles",
						"max": 0,
						"min": 0,
						"name": "code",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "jcmizkx8",
						"max": 0,
						"min": 0,
						"name": "description",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "0c8dwkfn",
						"max": null,
						"min": null,
						"name": "progress",
						"onlyInt": false,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "hrriffh0",
						"maxSelect": 1,
						"name": "type",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"single",
							"multi"
						]
					},
					{
						"hidden": false,
						"id": "ffmqerej",
						"maxSize": 2000000,
						"name": "coordinates",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "json"
					},
					{
						"hidden": false,
						"id": "tg5jbhns",
						"maxSize": 2000000,
						"name": "aggregates",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "json"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "rupq6yj561mghrr",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_QjY4Y2c` + "`" + ` ON ` + "`" + `maps` + "`" + ` (\n  ` + "`" + `territory` + "`" + `,\n  ` + "`" + `code` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_O2TlLJr` + "`" + ` ON ` + "`" + `maps` + "`" + ` (` + "`" + `territory` + "`" + `)"
				],
				"listRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation",
				"name": "maps",
				"system": false,
				"type": "base",
				"updateRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"viewRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) ||(@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= id && @collection.assignments:link.expiry_date > @now)"
			},
			{
				"createRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map && @collection.assignments:link.expiry_date > @now)",
				"deleteRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "relation2104863268",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": true,
						"collectionId": "rupq6yj561mghrr",
						"hidden": false,
						"id": "lb92rmc2",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "map",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "j7t6dxkw",
						"max": 0,
						"min": 0,
						"name": "message",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "lc9jrc1o",
						"max": 0,
						"min": 0,
						"name": "created_by",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "hzogq6vi",
						"name": "read",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "fhdf7arz",
						"name": "pinned",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "bsza9gbi",
						"maxSelect": 1,
						"name": "type",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"publisher",
							"conductor",
							"administrator"
						]
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "r2gqvjai7gbzl7a",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_dHQST9K` + "`" + ` ON ` + "`" + `messages` + "`" + ` (\n  ` + "`" + `map` + "`" + `,\n  ` + "`" + `pinned` + "`" + `,\n  ` + "`" + `created` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_4xBZkdzeoM` + "`" + ` ON ` + "`" + `messages` + "`" + ` (\n  ` + "`" + `map` + "`" + `,\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `pinned` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_IFmnqzx737` + "`" + ` ON ` + "`" + `messages` + "`" + ` (\n  ` + "`" + `map` + "`" + `,\n  ` + "`" + `type` + "`" + `,\n  ` + "`" + `read` + "`" + `\n)"
				],
				"listRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map && @collection.assignments:link.expiry_date > @now)",
				"name": "messages",
				"system": false,
				"type": "base",
				"updateRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"viewRule": null
			},
			{
				"createRule": null,
				"deleteRule": null,
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "mesd36gy",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "8r3y9ys9",
						"max": 0,
						"min": 0,
						"name": "code",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "tssb1qmr",
						"max": 0,
						"min": 0,
						"name": "description",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "jnyrgh8q",
						"name": "is_countable",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "96m2zgww",
						"name": "is_default",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "bool"
					},
					{
						"hidden": false,
						"id": "cknekhpo",
						"max": null,
						"min": null,
						"name": "sequence",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "wz7avhl19otivv6",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_oBkThEt` + "`" + ` ON ` + "`" + `options` + "`" + ` (\n  ` + "`" + `congregation` + "`" + `,\n  ` + "`" + `sequence` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_LDPjOnA` + "`" + ` ON ` + "`" + `options` + "`" + ` (\n  ` + "`" + `congregation` + "`" + `,\n  ` + "`" + `is_default` + "`" + `\n)"
				],
				"listRule": "(@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.congregation ?= congregation && @collection.assignments:link.expiry_date > @now)",
				"name": "options",
				"system": false,
				"type": "base",
				"updateRule": null,
				"viewRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation"
			},
			{
				"createRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"deleteRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "flm2xtzrt82ltf7",
						"hidden": false,
						"id": "ye2rjidt",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "user",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "5g34l9g3",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"hidden": false,
						"id": "ttkyanqt",
						"maxSelect": 1,
						"name": "role",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "select",
						"values": [
							"read_only",
							"conductor",
							"administrator"
						]
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "xln2af1in0pdo30",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_u9wr0mg` + "`" + ` ON ` + "`" + `roles` + "`" + ` (\n  ` + "`" + `congregation` + "`" + `,\n  ` + "`" + `role` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_iPooFW46s8` + "`" + ` ON ` + "`" + `roles` + "`" + ` (` + "`" + `user` + "`" + `)"
				],
				"listRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation",
				"name": "roles",
				"system": false,
				"type": "base",
				"updateRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"viewRule": null
			},
			{
				"createRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"deleteRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"fields": [
					{
						"autogeneratePattern": "[a-z0-9]{15}",
						"hidden": false,
						"id": "text3208210256",
						"max": 15,
						"min": 15,
						"name": "id",
						"pattern": "^[a-z0-9]+$",
						"presentable": false,
						"primaryKey": true,
						"required": true,
						"system": true,
						"type": "text"
					},
					{
						"cascadeDelete": true,
						"collectionId": "zzljam3htisq5tv",
						"hidden": false,
						"id": "jrmzerem",
						"maxSelect": 1,
						"minSelect": 0,
						"name": "congregation",
						"presentable": false,
						"required": false,
						"system": false,
						"type": "relation"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "ciqyzr1i",
						"max": 0,
						"min": 0,
						"name": "code",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"autogeneratePattern": "",
						"hidden": false,
						"id": "k9gj6j8e",
						"max": 0,
						"min": 0,
						"name": "description",
						"pattern": "",
						"presentable": false,
						"primaryKey": false,
						"required": false,
						"system": false,
						"type": "text"
					},
					{
						"hidden": false,
						"id": "5wijb8gx",
						"max": 100,
						"min": 0,
						"name": "progress",
						"onlyInt": true,
						"presentable": false,
						"required": false,
						"system": false,
						"type": "number"
					},
					{
						"hidden": false,
						"id": "autodate2990389176",
						"name": "created",
						"onCreate": true,
						"onUpdate": false,
						"presentable": false,
						"system": false,
						"type": "autodate"
					},
					{
						"hidden": false,
						"id": "autodate3332085495",
						"name": "updated",
						"onCreate": true,
						"onUpdate": true,
						"presentable": false,
						"system": false,
						"type": "autodate"
					}
				],
				"id": "kyfdlowtckhj9wm",
				"indexes": [
					"CREATE INDEX ` + "`" + `idx_fMh5sfU` + "`" + ` ON ` + "`" + `territories` + "`" + ` (\n  ` + "`" + `congregation` + "`" + `,\n  ` + "`" + `code` + "`" + `\n)",
					"CREATE INDEX ` + "`" + `idx_Otsl0yR` + "`" + ` ON ` + "`" + `territories` + "`" + ` (` + "`" + `congregation` + "`" + `)"
				],
				"listRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation",
				"name": "territories",
				"system": false,
				"type": "base",
				"updateRule": "@request.auth.id != \"\" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'",
				"viewRule": null
			}
		]`

		return app.ImportCollectionsByMarshaledJSON([]byte(jsonData), true)
	}, func(app core.App) error {
		return nil
	})
}
