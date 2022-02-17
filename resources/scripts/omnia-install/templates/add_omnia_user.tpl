---

- name: Create Omnia User
  hosts: localhost
  vars:
    username: ${username}
  tasks:
  - name: Create user omnia
    user:
      name: "{{ username }}"
  - name: Allow '{{ username }}' user to have passwordless sudo
    lineinfile:
      dest: /etc/sudoers
      state: present
      regexp: '^%%{{ username }}'
      line: '%%{{ username }} ALL=(ALL) NOPASSWD: ALL'
