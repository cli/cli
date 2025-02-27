Top-level configuration options
Required Fields

name (String): The template's name. Must be unique across all templates, including Markdown templates.
about (String): A description of this template's intended use. This will be shown in the issue template chooser interface.
Optional Fields

assignees (Array or String): This issue will be automatically assigned to these users. Can be array of usernames or comma-delimited string, e.g. "monalisa,nat"
labels (Array or String): This issue will automatically receive these labels upon creation. Can be array of labels or comma-delimited string, e.g. "bug,needs-triage"
title (String): Default title that will be pre-populated in the issue submission form.
type (String): This issue will be automatically assigned an issue type. Issue types are defined at the organization level and can be used to create a shared syntax across repos.
body (Array): Definition of user inputs.
Input type configuration options
Markdown
Markdown blocks contain arbitrary text that a maintainer can add to a template, to provide extra context or guidance to a contributor. Supports Markdown formatting. This text will not be rendered in the submitted issue body.

Required Fields

value (String): The text that will be rendered. Markdown formatting is supported.
Tip #1: YAML processing will cause the hash symbol to be treated as a comment. To insert Markdown headers, wrap your text in quotes.

Tip #2: For multi-line text, you can use the pipe operator.

Example
body:
- type: markdown
  attributes:
    value: "## Welcome!"
- type: markdown
  attributes:
    value: |
      Thanks for taking the time to fill out this bug! If you need real-time help, join us on Discord.
Input
Inputs are single-line form input fields. Contributors may use markdown formatting in their responses.

Required Attributes

label (String): A brief description of the expected user input.
Optional Attributes

description (String): Extra context or guidance about filling out this form input. Supports Markdown.
placeholder (String): Renders as semi-transparent "placeholder" element in the input field when it's empty.
value (String): Default text that is pre-populated in the input field.
ID

id (String): Optional unique identifier. Can only contain alphanumeric characters, -, and _.
Validations

required (Boolean): If true, the form will not be submittable until this is filled out.
Example
body:
- type: input
  id: prevalence
  attributes:
    label: Bug prevalence
    description: "How often do you or others encounter this bug?"
    placeholder: "Whenever I visit the user account page (1-2 times a week)"
  validations:
    required: true
Textarea
Very similar to inputs, textareas are multiple-line form input fields. Typically used if you'd like a contributor to provide an answer longer than a few words. Contributors may use markdown formatting in their responses.

Required Attributes

label (String): A brief description of the expected user input.
Optional Attributes

description (String): Extra context or guidance about filling out this form input. Supports Markdown.
placeholder (String): Renders as semi-transparent "placeholder" element in the input field when it's empty.
value (String): Default text that is pre-populated in the input field.
render (String): If a value is provided, user-submitted text will be formatted into a codeblock automatically.
ID

id (String): Optional unique identifier. Can only contain alphanumeric characters, -, and _.
Validations

required (Boolean): If true, the form will not be submittable until this is filled out.
Example
body:
- type: textarea
  id: repro
  attributes:
    label: Reproduction steps
    description: "How do you trigger this bug? Please walk us through it step by step."
    value: |
      1.
      2.
      3.
      ...
    render: bash
  validations:
    required: true
Dropdown
Users can select their answer from options defined by the maintainer.

Required Attributes

label (String): A brief description of the expected user input.
options (String Array): Set of values that user can select from to answer. Cannot be empty, and all choices must be distinct.
Optional Attributes

description (String): Extra context or guidance about filling out this form input. Supports Markdown.
multiple (Boolean): If true, users can submit multiple selections.
ID

id (String): Optional unique identifier. Can only contain alphanumeric characters, -, and _.
Validations

required (Boolean): If true, the form will not be submittable until at least one choice is selected.
Example
body:
- type: dropdown
  id: download
  attributes:
    label: How did you download the software?
    options:
      - Homebrew
      - MacPorts
      - apt-get
      - Built from source
  validations:
    required: true
Checkboxes
A group of one or more checkboxes. This will be saved as a Markdown checkbox, and will continue to support interactive updating.

Required Attributes

options (Array): Set of values that user can select from to answer. Cannot be empty. Each item must have a label, described below.
Optional Attributes

label (String): A brief description of the expected user input.
description (String): Extra context or guidance about filling out this form input. Supports Markdown.
ID

id (String): Optional unique identifier. Can only contain alphanumeric characters, -, and _.
Within each item in options, the following fields are supported:

Required

label (String): The text that will appear beside the checkbox. Markdown is supported for bold or italic text formatting, and hyperlinks.
Optional

required (Boolean): If required, the form will not be submittable unless checked.
Example
body:
- type: checkboxes
  id: cat-preferences
  attributes:
    label: What kinds of cats do you like?
    description: You may select more than one.
    options:
      - label: Orange cat (required. Everyone likes orange cats.)
        required: true
      - label: **Black cat**
