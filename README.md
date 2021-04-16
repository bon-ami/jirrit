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

  As shown in example.xml, root name is **jirrit**.<BR>
  **log** file name can be configured.<BR>
  Server types are **JIRA** and **Gerrit**.<BR>
  Names need to be unique within each type.<BR>
  **url** is needed for every server. **pass** can be provided, if not same as overall config.

  Three kinds of passwords can be configured.
  - **basic** is the plain text password.
  - **plain** is coded text password. Tools such as Postman can show this in the output message.
  - **digest** is generated by a server.

  For Jira servers, there may be more to config for issue closure with some fields filled.<BR>
Usually, these fields can be seen in an issue's detail.
  - **rejectrsn** is the field name for reject reasons.
  - **testpre** is the field name for test condition.
  - **teststep** is the field name for test steps.
  - **testexp** is the field name for test expectation.

  For Gerrit servers, there may be more to config for scores.
  - **score** contains the field name other than Code-Review and Verified, that needs +1. In my case, it is `Manual-Testing`.

## Actions

- Jira
  - transfer a case to someone
  - move status of a case
  - show details of a case
  - list comments of a case
  - add a comment to a case
  - list my open cases
  - link a case to the other
  - reject a case from any known statues
  - close a case to resolved from any known statues (change it to resolved)
  - close a case with default design as steps (change it to resolved, adding test condition="none", steps="default design" and expectation="none")
  - close a case with general requirement as steps (change it to resolved, adding test condition="none", steps="general requirement" and expectation="none")

- Gerrit
  - list merged submits of someone
  - list my open submits
  - list sb.'s open submits
  - list all my open revisions/commits
  - list all open submits
  - show details of a submit (by commit ID or change ID)
  - show reviewers of a submit
  - show revision/commit of a submit
  - rebase a submit
  - merge a submit
  - add scores to a submit (Code-Review +2, Verified +1, and Manual-Testing, or other field as configured, +1)
  - add socres, wait for it to be mergable and merge a submit
  - add socres, wait for it to be mergable and merge sb.'s submits
  - abandon all my open submits
  - abandon a submit
  - cherry pick all my open submits
  - cherry pick a submit

## Input grammar

 - For most prompts, [Enter] without default value as shown is taken as an invalid input and return to previous menu.
 - If previous value is in the format of ".+\-[0-9]+", or X-0, to be easier to read, and the new input is just a number, it will be taken as the number replacing the previous number part.
 - In some cases, input support multiple lines. End an input with "\" to indicate it is a line of multiple ones and continue inputting.
 - In some cases,<BR>
Input ".+\-[0-9]+[,][,][0-9]+", or "X-0,,1", or "0,,1", to be easier to read, to batch process all the ID's between, and including, the two numbers.<BR>
Input "X-0,Y-1,2", to be easier to read, to batch process all the ID's listed, adding previous letter part. ("X-0,Y-1,2" will result in processing X-0, Y-1 and Y-2.)<BR>
Supported for: reject and closure in Jira; rebase, merge and abandon in Gerrit.
