from typing import List

import openai
from tenacity import retry, stop_after_attempt, wait_random_exponential
import numpy as np

@retry(wait=wait_random_exponential(min=1, max=20), stop=stop_after_attempt(6))
def get_embeddings(
    list_of_tokens: List[int], engine="text-embedding-ada-002"
) -> List[np.ndarray]: #List[float]
    assert len(list_of_tokens) <= 2048, "The batch size should not be larger than 2048."

    # replace newlines, which accoring to OpenAI can negatively affect performance.
    # list_of_tokens = [text.replace("\n", " ") for text in list_of_tokens]
    ## we did this earlier, during tokenization

    data = openai.Embedding.create(input=list_of_tokens, engine=engine).data
    data = sorted(data, key=lambda x: x["index"])  # maintain the same order as input.
    return [np.array(d["embedding"]) for d in data]
