# agru

**a**nsible-**g**alaxy **r**equirements **u**pdater is very simple program, almost bash one-liner.

It reads ansible-galaxy's requirements.yml file, checks if the listed git repos has newer tags than presented in your file and updates the file if there are any newer tags.

```
Usage of agru:
  -r string
    	ansible-galaxy requirements file (default "requirements.yml")
```
