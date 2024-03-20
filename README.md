# alerts-tf-scrape
Generates Terraform for all NR1 alert policies and conditions in a given account


## To build
```
go build
```

## To configure
First you need to set two environment variables:
```
export NEW_RELIC_ACCOUNT=YOUR_TARGET_ACCOUNT_ID
export NEW_RELIC_USER_KEY=YOUR_USER_API_KEY
```

## To run
By default the scraper will run without concurrency, using a single Chrome window.
You can increase this with the `CONCURRENT` environment variable.  For example:
```
export CONCURRENT=8
```
The max setting is 20 based on API limits.

Then run as follows
```
./alerts-tf-scrape
```

The scraper will open a Chrome browser, and have you login to NR.
If you have a different SSO or IDP page for login, open a second tab and go to that page instead to authenticate.
Then, enter your email address on the first tab, and it will recognize that you are already logged in.

The log lines on your terminal will show you the progress as it proceeds
```
% ./alerts-tf-scrape 
2024/02/09 18:50:30 Parsing GraphQl policies response 944 bytes
2024/02/09 18:50:30 Parsing GraphQl conditions response 3701 bytes
2024/02/09 18:50:30 Launching Chrome web scraper
New Relic login page, please log in
2024/02/09 18:50:51 Login complete
2024/02/09 18:50:54 Walking 1 policies to generate Terraform
2024/02/09 18:50:54 Navigate to condition builder for "Container CPU Usage % is too high"
2024/02/09 18:50:58 Click [View as code] button
2024/02/09 18:50:58 Click [Terraform] Code preview
2024/02/09 18:50:58 Copied 771 bytes of TF code
2024/02/09 18:50:58 Writing alert policy terraform to policy_773015.tf
2024/02/09 18:50:59 Done
```

## Troubleshooting
When running higher concurrency, at times the browser may get Chrome error 5.
Refresh the window with command-R and it should continue.
