# vaultchuck

A one-off tool to ease migration from static credentials to Vault.

## usage

First, copy your current pipeline config to a file, like `pipeline-old.yml`.

Second, manually change the parameters in your pipeline config to how you want
them to look with vault. For example, you may change...:

```yaml
- name: concourse-docs
  type: cf
  source:
    api: ((cf-target))
    username: ((cf-username))
    password: ((cf-password))
    organization: ((cf-organization))
    space: ((cf-space))
```

...to...:

```yaml
- name: concourse-docs
  type: cf
  source:
    api: ((docs_cf.target))
    username: ((docs_cf.username))
    password: ((docs_cf.password))
    organization: ((docs_cf.organization))
    space: ((docs_cf.space))
```

Then, place your old params file somewhere on disk (ideally `/tmp`), and run:

```sh
vaultchuck --before pipeline-old.yml --after pipeline.yml --params /tmp/params.yml
```

You can pass `--dry-run` to see what it'll try to write (excluding the
credentials).

This will diff the pipelines, and at each delta, use the old pipeline to fetch
the credential, and use the new pipeline to determine where to build it.

Once all the new keys and fields are collected, it will write each one to
Vault. You can then commit and push your new pipeline.
