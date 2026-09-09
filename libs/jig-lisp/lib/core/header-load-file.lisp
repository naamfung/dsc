;; $MODULE header-load-file

(defn load-file [file-path]
    (eval
        (read-string
            (str
                ";; $MODULE " file-path "\n"
                "(do " (slurp file-path) "\n)"))))
