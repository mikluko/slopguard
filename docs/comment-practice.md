# What published practice says about the comments slopguard flags

## What was searched, and what could not be reached

Thirteen style guides were fetched directly (Google C++/Python/Java/Shell, Google eng-practices, Linux kernel `CodingStyle`, LLVM, Chromium, PEP 8, PEP 257, Go, Rust RFC 1574, Oracle Javadoc). Four practitioner books were read in the primary text: Ousterhout chapters 12-18, Kernighan and Pike section 1.6, McConnell's own redistributable chapter-32 checklist, and Martin chapter 4 through a critical edition that reproduces his sentences. Empirical work was pulled from ACM/IEEE/arXiv and author pages: Steidl 2013, Padioleau 2009, Pascarella 2017 and 2019, Potdar 2014, Bavota 2016, Maldonado 2017, Zampetti 2018, Wen 2019, Maalej 2013, Hu 2022, Rani 2021 and 2023, Jabrayilzade 2022, Oztas 2025, plus the 2024-2026 strand on AI-written code.

Not reached, and therefore not claimed: the Oracle Javadoc page returns 403 and was read through a text-extraction proxy (wording consistent across three passes, but not diff-verified against raw HTML); Fluri 2007/2009 and Ibrahim 2012 exist here only as quoted inside Wen 2019 and are marked secondhand throughout; the iComment paper body was not opened, only its abstract and the authors' page; McConnell's chapter-32 prose was not opened, only his checklist, so his six-way "kinds of comments" taxonomy is reported below from the checklist plus secondary reproductions and is flagged where it matters. One external note on method: a PDF summarizer fabricated an entire numeric table for the GitClear report during this review; every number below was read off a page, not off a summary.

Claims about slopguard's own behaviour are measured, not inferred. slopguard was built at `3f3f5c0` with ONNX Runtime present and run over the Go standard library (4,065 files, reproducing the repo's own figure of 1,034 findings exactly) and over probe files quoting the literature verbatim. Where a statement is an inference rather than a measurement or a quotation, the sentence says so.

---

## 1. Which of slopguard's classes has support in published practice

### `echo`: the strongest supported class, and independently reinvented

Steidl, Hummel and Juergens define a **coherence coefficient** that is the same metric slopguard's `echo` rule computes, at the same threshold. Theirs: *"The comparison counts how many words from one set correspond to a similar word in the other set... The c_coeff metric denotes the number of corresponding words divided by the total number of comment words."* Their hypothesis: *"Comments with c_coeff > 0.5 are trivial as they do not contain additional information."* ([Quality Analysis of Source Code Comments, ICPC 2013](https://teamscale.com/publications/2013-quality-analysis-of-source-code-comments.pdf)) slopguard's `echo` fires on `hits*2 >= len(words)`, which is the same 0.5 cut over comment content words (`internal/rule/echo.go:76`).

This is the only rule in slopguard with a published human validation. Steidl put it to 16 developers, all with five or more years of experience: for `c_coeff > 0.5`, *"9/10 voted trivial"* and *"In all nine cases, the agreement was with above 80% very strong."*

Four independent sources name the same defect:

- Ousterhout, red flag box: *"If the information in a comment is already obvious from the code next to the comment, then the comment isn't helpful. One example of this is when the comment uses the same words that make up the name of the thing it is describing."* (*A Philosophy of Software Design*, §13.2, p.104; read at [milkov.tech/assets/psd.pdf](https://milkov.tech/assets/psd.pdf))
- Kernighan and Pike: *"Don't belabor the obvious. Comments shouldn't report self-evident information, such as the fact that i++ has incremented i."* and, of their four examples, *"All of these comments should be deleted; they're just clutter."* (*The Practice of Programming*, §1.6, p.23; read at [theswissbay.ch](https://theswissbay.ch/pdf/Gentoomen%20Library/Software%20Engineering/B.W.Kernighan,%20R.Pike%20-%20The%20Practice%20of%20Programming.pdf))
- Google C++: *"Do not state the obvious. In particular, don't literally describe what code does, unless the behavior is nonobvious to a reader who understands C++ well."* ([styleguide](https://google.github.io/styleguide/cppguide.html))
- Google Python: *"On the other hand, never describe the code."* ([pyguide](https://google.github.io/styleguide/pyguide.html))

Base rate, measured: Jabrayilzade labelled 2,447 inline comments across eight Java and Python projects and found **"Obvious" at 31.0%**, the most common smell in all eight projects, range 16.5% to 52.3%. His definition: *"Comments that restate what the code does without giving additional information are considered obvious comments."* ([Taxonomy of Inline Code Comment Smells, Bilkent MSc 2022](https://repository.bilkent.edu.tr/bitstreams/412fe0ea-de2a-4a7d-a412-224fd95c35e3/download), journal version [EMSE 2023](https://doi.org/10.1007/s10664-023-10425-5))

**Verdict: supported, by more independent sources than any other class, with a measured base rate around 31% and one published human validation of the exact metric.**

### `tautology`: the idea is supported; the unary implementation is not what anyone described

Every source states the test **relationally**, as a comparison between the comment and the code beside it. Ousterhout gives it as an operational question: *"After you have written a comment, ask yourself the following question: could someone who has never seen the code write the comment just by looking at the code next to the comment?"* (§13.2, p.103). And: *"Comments at the same level as the code are likely to repeat the code."* (§13.3, p.104-105)

slopguard's `echo` implements that relational test. `tautology` does not: it embeds the comment's prose alone and scores it against a fitted direction, with the code entering only as a gate on shape (`internal/rule/rule.go:105` requires `restates()`, i.e. one line below and not a doc comment). The tool's own source concedes the point: *"Whether a comment restates the code is a fact about the pair, not about the comment. 'increment the counter' is a restatement above `n++` and a fair summary above a forty-line loop, so the text alone cannot tell them apart"* (`internal/rule/echo.go:13`).

The one paper that measured how well an automated judge does at this class is not encouraging. Oztas, Torun and Tüzün put GPT-4 on Jabrayilzade's taxonomy: accuracy rises from **34% to 55% when the code is supplied alongside the comment**, and F1 on the "Obvious" class reaches only **0.39**; a Random Forest over engineered features beats it at 69% accuracy and F1 0.53 on that class. ([Towards Automated Detection of Inline Code Comment Smells, EASE 2025](https://arxiv.org/html/2504.18956v2))

**Verdict: the class is real and well attested, but slopguard implements as a property of prose what every source defines as a property of a pair. `echo` is the faithful implementation; `tautology` is an approximation the literature does not endorse. On the standard library it supplies 249 of 1,034 findings against `echo`'s 623.**

### `leftover`: supported, uncontroversial, with measured base rates

- Steidl: *"(In general, we consider code comments to have negative impacts as they do not provide any information.)"*
- LLVM: *"Commenting out large blocks of code is discouraged, but if you really have to do this (for documentation purposes or as a suggestion for debug printing), use `#if 0` and `#endif`."* ([LLVM Coding Standards](https://llvm.org/docs/CodingStandards.html)) Note that the sanctioned form is not a comment and so escapes slopguard entirely; only the discouraged form is caught.
- Pascarella measures "Commented code" at **2.4% of OSS comment lines** and **5.9% in industrial code** ([MSR 2017](https://sback.it/publications/msr2017a.pdf), [EMSE 2019](https://repository.tudelft.nl/file/File_7b5835db-7701-48d9-9f6e-c63fb72db0c9)). Jabrayilzade measures it at 1.6% of inline comments.
- Martin lists "Commented-Out Code" among his bad-comment categories.

**Verdict: supported. Small but real, and slopguard's 142 stdlib findings sit in the right order of magnitude against Pascarella's 2.4%.**

### `compat`: the weakest class, and the one its most direct source argues against

No style guide in the corpus names "justifies the symbol by its own history" as a defect. Searched for explicitly: Google C++/Python/Java/Shell, eng-practices, Linux kernel, LLVM, Chromium, PEP 8, PEP 257, Go, Rust RFC 1574, Oracle Javadoc. Only two say anything adjacent, and both are about **how to write the notice, not about suppressing it**:

- Go: *"Paragraphs starting with `Deprecated:` are treated as deprecation notices."* ([go.dev/doc/comment](https://go.dev/doc/comment))
- Oracle: *"The @deprecated description in the first sentence should at least tell the user when the API was deprecated and what to use as a replacement."*

Against it, three sources actively require the shape:

- McConnell's own checklist asks: *"Is code that works around an error or undocumented feature commented?"* ([Code Complete 2 checklists, ch.32](https://www.matthewjmiller.net/files/cc2e_checklists.pdf), a document McConnell licenses for redistribution). That is the `compat` shape stated as an obligation.
- Ousterhout §16.3 is discussed in section 4 below and is the sharpest contradiction in this review.
- Kernighan and Pike: *"It may also be valuable to suggest why particular decisions were made."* (§1.6, p.24)

Prevalence: Pascarella measures the **Deprecation** category at **0.2% of comment lines** in both OSS and industrial corpora, the smallest of his sixteen subcategories. slopguard produces 20 `compat` findings across 4,065 standard-library files, 1.9% of its own output.

Measured precision, five inspected of those 20: four correctly name a compatibility retention (`runtime/pprof/pprof.go:606` *"It is preserved for backwards compatibility."*; `net/net.go:639` *"(No longer used; kept for compatibility.)"*; `go/types/api.go:101`; `encoding/json/decode.go:146`), one is a step comment misread (`cmd/compile/internal/ssa/decompose.go:429`, `// Get rid of obsolete names`). One of the four true positives is instructive: `encoding/json/decode.go:146` reads *"Deprecated: No longer used; kept for compatibility."* The nudge tells the author to *"state the contract instead, as a Deprecated: note if that is what it is"* on a comment that already is one.

Robustness, measured on four minimal pairs differing only in a trailing full stop:

```
// kept for backwards compatibility                 fires at 0.080
// kept for backwards compatibility.                fires at 0.030
// legacy field, do not use it in new code          fires at 0.045
// legacy field, do not use it in new code.         fires at 0.010
// kept for backwards compatibility with older callers    fires at 0.067
// kept for backwards compatibility with older callers.   does not fire
```

A period costs roughly half the class's margin, and the class operates entirely within a 0.01-0.13 band, so punctuation flips the verdict at the edge. Go's own convention mandates the period: *"Comments documenting declarations should be full sentences... Comments should begin with the name of the thing being described and end in a period."* ([Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)) Of the three examples the README gives for this class, one (*"left in place so the rollout can be reverted in one step"*) does not fire at all.

**Verdict: unsupported by any style guide, contradicted by McConnell and Ousterhout, targeting a category measured at 0.2% of comment lines, and calibrated on a surface form that Go's own convention forbids. This is the class to cut or rebuild.**

### `padding` / `hollow`: no support anywhere, and contrary evidence

**No guide in the corpus states a numeric limit on comment length.** The three near-misses are formatting rules, not verbosity rules: LLVM's single-sentence rule applies to the Doxygen `\brief` line only; PEP 257's *"One-liners are for really obvious cases. They should really fit on one line."* is about one-liner docstrings; Oracle's *"limit any doc-comment lines to 80 characters"* is line wrapping. Every other guide: nothing.

Steidl tested a length rule empirically and rejected it. Long inline comments were **kept** by developers *"with a very high agreement among each other of at least 88% in nine out of ten cases"*, and the paper concludes: *"It is inappropriate to conclude that inline comments should contain at least thirty words. In general, they should not."* For the middle band, *"the survey did not reveal a clear programmers' preference how to handle comments."*

Practitioners rank verbosity last among comment problems. Hu et al. surveyed **720 practitioners**: lack of comments 69%, generic comments 62%, outdated comments 47%, code/comment inconsistency 31%, redundant comments 27%, **too-long comments 16%** ([Practitioners' Expectations on Automated Code Comment Generation, ICSE 2022](https://xin-xia.github.io/publication/icse224.pdf)).

And the judgment the rule automates is one humans barely agree on. Maalej and Robillard had 17 coders double-code 5,574 documentation units; on the **Non-information** variable (*"documentation containing any complete sentence or self-contained fragment of text that provides only uninformative boilerplate text"*) raw agreement was **0.706 against a chance baseline of 0.608**, with disagreement rates of **26.4% (JDK) and 32.5% (.NET), rising to 36.0% on member documentation** ([Patterns of Knowledge in API Reference Documentation, TSE 2013](https://www.cs.mcgill.ca/~martin/papers/tse2013a.pdf)).

Measured: `hollow` produced **0 findings across 4,065 standard-library files**, and 0 on Google's mandated Shell function-header form.

**Verdict: unsupported. The repo already deleted the sentence-counting version for good internal reasons; the external evidence says the replacement is aimed at a problem practitioners rank last and that expert humans cannot reliably agree on. It currently fires on nothing.**

### The removed change-event class — the sources disagree, so the removal was not settled by evidence

The repo removed this class because directions fitted from disjoint halves agreed at only cos +0.32. That is a sound reason to remove an unstable classifier. It is not evidence the class was wrong, and the literature is split on whether it is a defect at all:

- **Against the comment**: Martin's "Journal Comments" category condemns changelog blocks in source, on the ground that version control now holds that information.
- **For the comment**: Ousterhout §16.3 (see section 4) is titled *"Comments belong in the code, not the commit log"* and argues the opposite directly.
- **Measured**: Wen et al. found the documented reasons comments get deleted are outdatedness and debt-repayment, not history-recording ([ICPC 2019](https://csnagy.github.io/research/pdfs/2019/Wen2019-preprint.pdf)).

**Verdict: the class is contested in the literature, not merely hard to fit. Reviving it would require taking a side against Ousterhout, which the tool's own nudge text already does without saying so.**

---

## 2. Classes published practice names that slopguard does not carry

Ranked by how many independent sources name them and by whether an embedding over one comment's prose could plausibly detect them. Coverage claims are measured: a probe file containing one instance of each of items 3-6 produced zero findings.

**1. A doc comment that restates its own signature.** Six independent sources, one validated metric, and slopguard structurally cannot fire on it.

- Ousterhout's red flag is illustrated with exactly three examples, all single-sentence doc comments over declarations: *"Obtain a normalized resource name from REQ."*, *"Downcast PARAMETER to TYPE."*, *"The horizontal padding of each line in the text."* He notes *"the only thing in the second comment that isn't in the code is the word 'to'!"* (§13.2, p.103)
- LLVM: *"Avoid restating the information that can be inferred from the API name or signature."* and *"Don't duplicate the function or class name at the beginning of the comment."*
- Oracle Javadoc: *"If the doc comment merely repeats the API name in sentence form, it is not providing more information."*
- PEP 257: *"The one-line docstring should NOT be a 'signature' reiterating the function/method parameters."*
- Linux kernel: *"Do not add boilerplate kernel-doc which simply reiterates what's obvious from the signature of the function."* ([kernel.org](https://www.kernel.org/doc/html/latest/process/coding-style.html))
- Steidl's validated `c_coeff` rule was defined **on member comments**, i.e. documentation, and measured trivial member comments at 1% to 9% per project. *"Member comments in particular should provide more information than just repeating the method name."*

slopguard excludes this population by construction. `echo` returns false for any doc comment (`internal/rule/echo.go:49`); `tautology` is gated on `restates()`, which does the same (`internal/rule/rule.go:105`); `hollow` requires two or more sentences and two or more hollow ones, so a one-sentence doc never reaches it. Measured: all three of Ousterhout's condemned doc-comment examples pass unflagged, and so do PEP 257-shaped docstrings reading `"""Increment the counter."""`, `"""Close the connection."""` and `"""Parse the response body."""` — the README's own three `tautology` examples, merely relocated above a declaration. Detectability is good and needs no new model: the signature-word machinery already exists in `declared()` (`internal/rule/padding.go:204`).

**2. Generic / uninformative comments.** Second-most-reported problem among 720 practitioners at 62% (Hu 2022); Maalej's "Non-information"; Steidl's usefulness criterion (*"If source code understanding was not harder with the comment being deleted, the comment would not be helpful."*). This is what `hollow` reaches for. Detectable from prose alone in principle, but the agreement data above (0.706 vs 0.608 chance) says the ceiling is low, and Oztas' F1 0.39 says the current state of the art is lower still.

**3. Incomplete comments.** Pascarella measures these at **7.1% of industrial comment lines**, up from 0.4% in OSS, and warns: *"INCOMPLETE and COMMENTED CODE subcategories could be an indication of bad practices and low readability and maintainability of code."* Large, measured, and plausibly detectable from prose (trailing ellipses, "for now", unfinished clauses). Not carried.

**4. Journal / changelog comments.** Martin's category. Structural, needs no model (date-prefixed line runs). Not carried; measured as not firing.

**5. Position markers and elaborate banners.** Martin's "Position Markers"; Kernighan and Pike condemn *"distracting the reader with elaborate typographical displays"* (§1.6, p.23). Purely structural. Not carried. Note the direct conflict: Steidl's taxonomy treats **Section** comments as a legitimate category, not a defect.

**6. Attribution and bylines, closing-brace comments, HTML in doc blocks.** Martin's categories. All structural, all cheap, all low-value now that version control exists. Not carried.

**Deliberately not recommended: TODO / self-admitted technical debt.** It is the best-measured comment class in the literature (Potdar and Shihab found SATD in 2.4%-31.0% of files across four systems; Bavota and Russo found ~0.3% of all comments across 159 projects and 658,026 commits, with **~25% of pattern matches being false positives**). But Google C++ and Google Java both *mandate* the form, so it is a tracked-debt marker rather than a defect, and slopguard is right to leave it alone.

---

## 3. Where the sources disagree

These are not averaged. Each is a live disagreement between sources of comparable standing.

**Are comments a failure, or the point?** Martin: *"Comments are always failures."* and *"Every time you write a comment, you should grimace and feel the failure of your ability of expression."* Ousterhout answers the underlying position by name: *"Some people believe that if code is written well, it is so obvious that no comments are needed. This is a delicious myth, like a rumor that ice cream is good for your health: we'd really like to believe it! Unfortunately, it's simply not true."* (§12.1, p.96). Google C++ takes Martin's side in passing: *"while comments are very important, the best code is self-documenting."* The published pushback on Martin is explicit: the critic calls it *"the book's second worst idea: comments are failure to write good code"*, and argues *"Code alone can never provide all context. Even perfect code can only show what's there - not what's deliberately excluded or what was considered and rejected"* and *"Declaring 'comments are failure' has led developers to avoid a crucial tool for design, abstraction, and documentation."* ([Clean Code critique, ch.4](https://bugzmanov.github.io/cleancode-critique/chapter_4.html); the critique's author is not named on the page).

**What, or why?** The two most widely enforced guides say opposite things in the same grammatical frame. Linux kernel: *"Generally, you want your comments to tell WHAT your code does, not HOW."* Google eng-practices: *"Usually comments are useful when they explain why some code exists, and should not be explaining what some code is doing."* ([looking-for.html](https://google.github.io/eng-practices/review/reviewer/looking-for.html)) McConnell's checklist asks both at once: *"Do comments explain the code's intent or summarize what the code does, rather than just repeating the code?"* and *"Do comments focus on why rather than how?"*

**Should a doc comment open by naming its symbol?** Go requires it: *"Comments should begin with the name of the thing being described."* LLVM forbids it: *"Don't duplicate the function or class name at the beginning of the comment."* slopguard sides with Go by exempting the first sentence in `hollow`, which is correct for Go and wrong for LLVM's C++.

**Are comments inside a function body acceptable?** Linux kernel: *"try to avoid putting comments inside a function body: if the function is so complex that you need to separately comment parts of it, you should probably go back to chapter 6 for a while."* Ousterhout prescribes the opposite and shows a worked example: *"This approach works particularly well if the first line after each blank line is a comment describing the next block of code: the blank lines make the comments more visible."* (§18.1, p.147). slopguard sides with the kernel by lowering the threshold inside bodies.

**Is summarising the code a defect?** McConnell's checklist explicitly licenses it (*"explain the code's intent **or summarize what the code does**"*), and Ousterhout's prescribed replacement for a bad comment is a summary at a higher level. slopguard's `tautology` reason string says *"restates what the code already says"* without distinguishing the two.

**Are banner/section comments legitimate?** Martin and Kernighan/Pike condemn them; Steidl's category model treats **Section** as an ordinary comment category alongside header and member comments.

---

## 4. Where published practice contradicts slopguard's premise

**The nudge tells agents to do the thing Ousterhout wrote a section to forbid.** slopguard's `additionalContext` says: *"If it only records this change, cut it and carry it into the commit message."* Ousterhout's §16.3 is titled *"Comments belong in the code, not the commit log"*, and reads:

> *"A common mistake when modifying code is to put detailed information about the change in the commit message for the source code repository, but then not to document it in the code... When writing a commit message, ask yourself whether developers will need to use that information in the future. If so, then document this information in the code. An example is a commit message describing a subtle problem that motivated a code change. If this isn't documented in the code, then a developer might come along later and undo the change without realizing that they have re-created a bug. If you want to include a copy of this information in the commit message as well, that's fine, but the most important thing is to get it in the code. This illustrates the principle of placing documentation in the place where developers are most likely to see it; the commit log is rarely that place."* (§16.3, p.138)

His worked example is precisely the `compat` shape: a note kept in the code so a later developer does not undo a change and re-create a bug. This is a head-on collision with the tool's stated doctrine, not a matter of emphasis. Note that the collision is with the **instruction text**, not with the classifier: measured, none of `// Do not remove this retry: without it the scheduler deadlocked...`, `// This bound is deliberately 512 and not 1024...` or `// Left in place so the rollout can be reverted in one step` fires. The tool advises against a practice it does not actually detect.

**Ousterhout's "different level of detail" test is sharper than `tautology`, and it makes the rule blunt rather than wrong.** His formulation is *"Comments augment the code by providing information at a different level of detail"* (§13.3, p.104), with the corollary *"Comments at the same level as the code are likely to repeat the code."* Level is a relation between two things. A rule reading prose alone cannot compute it. Concretely, his prescribed good comment — *"Try to append the current key hash onto an existing RPC to the desired server that hasn't been sent yet."* — is a summary of the loop beneath it and differs from the comment he condemns only in altitude.

Here the measurement is kinder to the tool than the argument predicted. A file containing seven comments Ousterhout explicitly prescribes (the Doxygen `Buffer::copy` contract, the four `allocAux` step comments, the phase-list comment, the improved `textHorizontalPadding` and `offset` declarations) produced **zero findings**. A file containing seven he explicitly condemns produced **two**, both `echo`, both on the scroll-bar pair. So `tautology` is not blunt in the false-positive direction; it is blunt in the recall direction, missing five of seven of the canonical bad cases, three of them because they are doc comments.

**Most real comments are neither contract nor slop.** slopguard's negative space is "everything that states a contract". Pascarella's measured distribution says that space is small. Of OSS comment lines: **License 39.4%**, Summary 25.5%, Usage 11.6%, Pointer 6.8%, Expand 6.9%, Ownership 3.5%, Directive 4.3%, Commented code 2.4%, **Rationale 2.0%**, Exception 1.2%, Deprecation 0.2%. The categories that read as contract (Rationale, Usage, Exception, Deprecation) total about 15%. His own conclusion: *"We see that 59% of lines of comments should not be considered (i.e., categories from C to F), as they do not reflect any aspect of the readability and maintainability of the code they pertain to"* and *"the SUMMARY accounts for only 24% of the overall lines of comments."* Padioleau's independent finding on OS code points the same way: *"While many comments are simply explanations of the code, we found that 52.6±2.9% of the comments from Linux, FreeBSD, and OpenSolaris are not merely explanations."* ([ICSE 2009](https://www.cs.purdue.edu/homes/lintan/publications/cComment_icse09.pdf))

**The tool judges placement, but placement is the rarest defect measured.** slopguard's framing is that a claim belongs somewhere else. In the only taxonomy with a placement category, **Non-local is 4 of 2,447 comments, 0.2%**, while Obvious is 31%. Misleading, the truth-condition class slopguard explicitly declines to judge, is 0.8%. On these numbers the tool's actual value comes from redundancy detection, which is what `echo` does, and the placement framing describes a phenomenon nobody has measured as common.

**The AI-slop premise the README opens with is not supported by the one study that measured it.** A study of 12,749 commits, 19,816 AI-generated files and 36,467 matched human files found comment ratios of **18.01% (AI) vs 17.96% (human)**, concluding *"The similar comment ratios (18.01% vs. 17.96%) indicate that AI-generated code does not rely more heavily on comments despite being larger in size."* ([arXiv:2603.27130v2](https://arxiv.org/html/2603.27130v2), values cross-checked across two fetches but summarizer-mediated, not raw-text verified). GitClear, the most-cited industry source on AI code quality, excludes comments from its metrics by construction: *"'Copy/Pasted' code... quantifies the frequency of non-keyword, non-autogenerated, non-comment lines of code"* ([2025 report](https://gitclear-public.s3.us-west-2.amazonaws.com/GitClear-AI-Copilot-Code-Quality-2025.pdf), read directly). No study compares redundancy or triviality of LLM-written against human-written comments under any comment-quality taxonomy; that gap is itself a finding. What is measured about AI comments is accuracy, not placement: roughly a fifth of one leading model's comments *"contained demonstrably inaccurate statements"* ([arXiv:2406.14836](https://arxiv.org/abs/2406.14836), abstract only) — a truth problem, which slopguard declines to judge.

**Three shapes the guides condemn are exempted by construction.** Kernighan and Pike's canonical worthless comment is `zerocount++; /* Increment zero entry counter */`, and their second block of condemned examples (*"skip white space"*, *"end of file"*, *"left paren"*) is entirely endline comments: *"These comments should also be deleted, since the well-chosen names already convey the information."* McConnell's checklist asks *"Does the code avoid endline comments?"* slopguard exempts every trailing comment from the rules that read it against its line, and its README acknowledges the cost. That exemption is defensible on its own evidence, but it means the tool spares the exact examples two of the four books use to introduce the problem.

**The one rule that can read documentation never fires.** Across 4,065 standard-library files, `hollow` produced zero findings, and `echo` and `tautology` are both barred from doc comments. The tool therefore says nothing about documentation at all, on any corpus, while five style guides and the one validated academic metric all locate the redundancy defect precisely there.

---

## Measurements taken for this review

| Probe | Result |
|---|---|
| Go standard library, 4,065 files | 1,034 findings: `echo` 623, `tautology` 249, `leftover` 142, `compat` 20, `hollow` 0. Reproduces `docs/design.md` exactly. |
| 7 comments Ousterhout prescribes (verbatim) | 0 findings |
| 7 comments Ousterhout condemns (verbatim) | 2 findings, both `echo`; the 3 doc-comment cases and the vague-initialiser case missed |
| PEP 257-shaped docstrings of the README's own 3 `tautology` examples | 0 findings |
| Google Shell mandated function-header block | 0 findings |
| Martin's journal / position-marker / closing-brace / byline / HTML categories | 0 findings |
| `compat`, 4 minimal pairs differing only by a trailing period | period costs ~half the score; flips the verdict at the margin |
| `compat`, 5 of 20 stdlib findings inspected | 4 correct, 1 step comment misread; 1 of the 4 already carried a `Deprecated:` marker |
