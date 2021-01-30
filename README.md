# jirrit

## Command line parameters

 - `-h` help message
 - `-v` logging enabled and more interactions. Most query actions need logging.
 - `-vv` verbose messages
 - `-vvv` verbose messages with network I/O
 - `-i string` provide an issue ID or assignee. Some actions are subjected to a certain issue or issues. Enter it when running or as a command param.
 - `-b string` provide a branch.
 - `-hd string` provide an new assignee for issue transfer.
 - `-p string` provide a component.
 - `-c string` provide a config file. It defaults to jirrit.xml under current dir or home dir.
 - `-l string` provide a log file. It defaults to jirrit.log under current dir.

## Config xml

  As shown in example.xml, root name is jirrit.
  An overall config of user name and password is shown.
  Server types are Jira and Gerrit.
  Names need to be unique within each type.
  url is needed for every server. pass can be provided, if not same as overall config.

  Three kinds of passwords can be configured.
  - basic is the plain text password.
  - plain is coded text password. Tools such as Postman can show this in the output message.
  - digest is generated by a server.

  For Jira servers, there may be more to config for issue closure with some fields filled.
Usually, these fields can be seen in an issue's detail.
  - testpre is the field name for test condition.
  - teststep is the field name for test steps.
  - testexp is the field name for test expectation.

  For Gerrit servers, there may be more to config for scores.
  - score contains the field name other than Code-Review and Verified, that needs +1. In my case, it is `Manual-Testing`.

## Actions

- Jira
  - transfer an issue
  - change an issue's state
  - show details of an issue
  - list comments of a case
  - add a comment to a case
  - list my open issues
  - link a case to the other
  - change an issue's states from open all the way to resolved
  - resolve an issue, adding test condition="none", steps="default design" and expectation="none"
  - resolve an issue, adding test condition="none", steps="general requirement" and expectation="none"
- Gerrit
  - list merged commits by branch and assignee
  - list my open commits
  - list an assignee's open commits
  - list all open commits
  - show details of an commit by commit ID or change ID
  - show current revision of a commit
  - show current revision of all my open commits
  - show reviewing scores of a commit
  - rebase a commit
  - merge a commit
  - wait for a commit to be mergable and merge it, adding scores when needed
  - for all open commits of a branch by an assignee, wait them to be mergable and merge them, adding scores when needed
  - Code-Review +2, Verified +1, (and Manual-Testing, or other field as configured, +1) to a commit
  - abandon all my open commits
  - abandon a commit
  - cherry pick all my open commits
  - cherry pick a commit 

## Input grammar

 - If previous field has a value, [Enter] without any characters will use that value.
 - If previous value is in the format of ".+\-[0-9]+", and the new input is just a number, it will be taken as the number replacing the previous number part.
 - In some cases, input support multiple lines. End an input with "\" to indicate it is a line of multiple ones and continue inputting.
 - In some cases, input ".+\-[0-9]+[,][0-9]+" to batch process all the ID's between, and including, the two numbers.
