# feedgen

As Part of the [DAY[0] Podcast](https://dayzerosec.com) I've found myself following a lot of feeds.

Unfortunately, many solid blogs do not provide feeds. I attempted to use a variety of services to generate feeds but was not too happy with most. While all the features I needed existed across several programs, none provided all the customizability I wanted. Notably parsing dates is pretty important as something I never want is to add a feed and find myself with a ton of entries added to the top of my aggregator because the date wasn't supported.

This isn't necessarily easy to use, but if you're familiar with CSS it shouldn't be too painful.

If you're only interested in the feeds that this already outputs, I generate all these feeds and currently output them to https://little-canada.org/feeds/output/ 

### General Usage

`./feedgen -f [feed generator type] -t [output type] -o [output file] -w [working directory] [-c config file]` 

**-f [feed generator type]**

There are currently five supported feed types:
 
 - `h1` which runs the Hackerone generator, which tracks the [latest disclosed Hacktivity](https://hackerone.com/hacktivity?querystring=&filter=type:public&order_direction=DESC&order_field=latest_disclosable_activity_at&followed_only=false) generator
 - `p0` which runs the ProjectZero generator, which tracks the [Project Zero issues list](https://bugs.chromium.org/p/project-zero/issues/list?q=&can=1&sort=-id). One important note about this one is that the feed does not grab dates so it is only accurate after it has been running. The first feed generated will be be based purely on ids.
 - `p0rca` which runs the Project Zero Root Cause Analysis generator, which tracks the [Project Zero Root Cause Analysis list](https://googleprojectzero.github.io/0days-in-the-wild/rca.html)
 - `pl` which runs the PentesterLand Writeups generator tracking the writeups published on [Pentester.Land](https://pentester.land/writeups/)
 - `css` is the more generic and useful option. It takes in a config file (json) which defines CSS selectors to match page elements that represent different parts of the feed.

**-t [output type]**

Multiple output formats are supported and can be selected:

 - `rss`
 - `atom`
 - `json`

**-o [output file]**

Output file is pretty much what is sounds like. Specify the file the feed will be written to. There is one special case:

 - `-` as a output file will write to stdout.

**-w [working directory]**

This just tells the program a specific folder to write any state-related files. At present only the ProjectZero generator uses this to track which issues it has seen.

**-c [config file]**

The configuration file is only necessary when using the CSS generator. This is how you tell the CSS generator what config to use.

### CSS Configuration

All CSS Configs must have three fields:

 - url - This defines the URL that will be requested
 - title - A title for the feed
 - item_selectors - This is a dictionary containing CSS selectors.

##### Item Selectors

Any of the following selectors can be provided.

 - container - This selector will basically be used as a prefix for all following selectors. It should match each individual item that will end up in the feed.
 - id
 - title
 - author
 - description 
 - content - *Content and Description both perform the same job.*
 - updated
 - created

If you use updated or created you must also provide a `updated_format` or `created_format`. These are passed directly in [time.Parse](https://golangbyexample.com/parse-time-in-golang/)

If you don't want to read the docs, the basic idea if you just write out the date for `01/02 03:04:05PM '06 -0700` (or in a more readable form: `Mon Jan 2 15:04:05 MST 2006`)

For example if a blog used the format: January 09, 2021 @ 9:01 am you would write the `_format` date as. `January 02, 2006 @ 3:04 am`

##### Writting Selectors

CSS isn't the greatest tool for the job, but when all you have is a hammer, everything looks like a nail. CSS has been quite capable of matching precisely what I need. `:first-of-type` `:nth-of-type(n)` and `:not(...negative selection here...)` have been very useful to know about.
