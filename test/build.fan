#! /usr/bin/env fan
//
// Copyright (c) 2026, Albireo Energy
// All Rights Reserved
//
// History:
//   27 Mar 26   Mike Melillo   Creation
//

using build

**
** Build: aeFdcimUtilsExt
**
class Build : BuildPod
{
  new make()
  {
    podName = "aeFdcimUtilsExt"
    summary = "TODO: summary of pod name..."
    version = Version("1.0")
    meta    = [
                "org.name":     "Albireo Energy",
                //"org.uri":      "http://acme.com/",
                //"proj.name":    "Project Name",
                //"proj.uri":     "http://acme.com/product/",
                "ext.depends": "event,rule,phIct",
                "license.name": "Commercial",
              ]
    depends = ["sys 1.0",
               "haystack 3.1",
               "folio 3.1",
               "axon 3.1",
               "skyarcd 3.1",
               "event 3.1",
               "rule 3.1",
               "phIct 3.9"]
    srcDirs = [`fan/`]
    resDirs = [`locale/`,
               `lib/`]
    index   =
    [
      "skyarc.ext": "aeFdcimUtilsExt::AeFdcimUtilsExt",
    ]
  }
}
