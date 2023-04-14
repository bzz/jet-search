# Semantic Code & Doc Search

Experiments with NN Embeddings for code.

 * [Code search over intellij-community/platform using GPT-3 embeddings](./gpt3_code_embeddings.ipynb)
 * [Documentation Q&A over intellij-community](./gpt3_doc_q_and_a.ipynb)


<details>

 1. Accuare the data from intellij-community
    Clone, parse and extract function declarations for Java and Kotlin

 2. Get the embeddings
    Embed all the functions using
     * OpenAI API for Embeddings
       using .jsonl and `request_parallel_processor.py`
     * CodeGen running localy on GPU
 
    Build an Index
     * Annoy
     * FAISS

 3. Code clustering
 
 4. Code Search
    Interactive queries over intellij-community

 5. Documentation Q&A

 6. Evaluation on CodeSearchNet Java
     * Embed (OpenAI, CodeGen)
     * Cluster
     * Query rephrasing \w in-context learning (few-shot)
     * Run evaluation (nDCG)
     * IR baseline

</details>